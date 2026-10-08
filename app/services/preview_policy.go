package services

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"ponta_drive/app/contracts"
)

// MaxPreviewProxyBytes bounds how large a file the server will proxy for
// preview. Files above it are offered as a download instead.
const MaxPreviewProxyBytes int64 = 50 * 1024 * 1024

// PreviewKind is how the preview pipeline treats a file.
type PreviewKind int

const (
	KindMedia PreviewKind = iota
	KindFetchBased
	KindUnsupported
)

// PreviewStrategy is the transport the client should use.
type PreviewStrategy string

const (
	StrategyDirect   PreviewStrategy = "direct"
	StrategyProxy    PreviewStrategy = "proxy"
	StrategyFallback PreviewStrategy = "fallback"
)

var mediaMimePrefixes = []string{"image/", "video/", "audio/"}

var mediaExtensions = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "gif": true, "webp": true, "bmp": true,
	"svg": true, "avif": true, "heic": true, "heif": true, "ico": true, "tiff": true, "tif": true,
	"mp4": true, "webm": true, "mov": true, "mkv": true, "avi": true, "m4v": true,
	"mp3": true, "wav": true, "ogg": true, "flac": true, "aac": true, "m4a": true, "opus": true,
}

var fetchBasedExtensions = map[string]bool{
	"pdf": true, "txt": true, "md": true, "markdown": true, "json": true, "csv": true,
	"xml": true, "html": true, "htm": true, "yaml": true, "yml": true, "log": true,
	"go": true, "js": true, "ts": true, "tsx": true, "jsx": true, "py": true, "rb": true,
	"java": true, "c": true, "h": true, "cpp": true, "cs": true, "php": true, "rs": true,
	"sh": true, "sql": true, "css": true, "scss": true, "vue": true, "swift": true, "kt": true,
	"doc": true, "docx": true, "xls": true, "xlsx": true, "ppt": true, "pptx": true,
	"odt": true, "ods": true, "odp": true, "rtf": true,
	"zip": true, "rar": true, "7z": true, "tar": true, "gz": true,
	"epub": true, "mobi": true, "azw3": true,
	"ttf": true, "otf": true, "woff": true, "woff2": true,
	"srt": true, "vtt": true, "ass": true, "ssa": true, "lrc": true,
	"msg": true, "dxf": true, "dwg": true,
}

// ClassifyPreviewKind decides how a file is previewed. It prefers the MIME type
// but falls back to the extension, which is what makes a file with an empty or
// generic MIME (application/octet-stream) still preview correctly.
func ClassifyPreviewKind(mimeType string, extension string) PreviewKind {
	mime := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(extension), "."))

	for _, prefix := range mediaMimePrefixes {
		if strings.HasPrefix(mime, prefix) {
			return KindMedia
		}
	}
	if mediaExtensions[ext] {
		return KindMedia
	}
	if mime == "application/pdf" || fetchBasedExtensions[ext] {
		return KindFetchBased
	}
	if strings.HasPrefix(mime, "text/") {
		return KindFetchBased
	}
	return KindUnsupported
}

// DecidePreviewStrategy picks the transport for a file.
//
// Media goes direct (its renderers do not need CORS, so a public or presigned
// URL always works). Fetch-based files are proxied while they fit the cap,
// because their renderers fetch and therefore need CORS; direct is used for them
// only via a reachable public URL, which is CORS-enabled by nature.
func DecidePreviewStrategy(kind PreviewKind, size int64, publicReachable bool) (PreviewStrategy, string) {
	switch kind {
	case KindMedia:
		return StrategyDirect, ""
	case KindFetchBased:
		if size <= MaxPreviewProxyBytes {
			return StrategyProxy, ""
		}
		if publicReachable {
			return StrategyDirect, ""
		}
		return StrategyFallback, "too_large"
	default:
		return StrategyFallback, "unsupported"
	}
}

// ParseRangeHeader parses a single-range "bytes=start-end" header. An empty
// header means the whole object. Multi-range requests are rejected because the
// proxy streams one contiguous span.
func ParseRangeHeader(header string, size int64) (int64, int64, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, size, nil
	}
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, errors.New("unsupported range unit")
	}
	spec := strings.TrimPrefix(header, "bytes=")
	if strings.Contains(spec, ",") {
		return 0, 0, errors.New("multiple ranges are not supported")
	}
	startStr, endStr, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, 0, errors.New("malformed range")
	}

	start, err := strconv.ParseInt(strings.TrimSpace(startStr), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("malformed range start: %w", err)
	}
	if start < 0 || start >= size {
		return 0, 0, errors.New("range start out of bounds")
	}

	end := size - 1
	if strings.TrimSpace(endStr) != "" {
		parsed, perr := strconv.ParseInt(strings.TrimSpace(endStr), 10, 64)
		if perr != nil {
			return 0, 0, fmt.Errorf("malformed range end: %w", perr)
		}
		if parsed < start {
			return 0, 0, errors.New("range end before start")
		}
		if parsed < end {
			end = parsed
		}
	}

	return start, end - start + 1, nil
}

// MergeCorsRules returns existing rules plus one that allows origin to GET and
// HEAD the bucket. It is idempotent: if a rule already covers origin for GET,
// nothing is added and changed is false.
func MergeCorsRules(existing []contracts.CORSRule, origin string) ([]contracts.CORSRule, bool) {
	for _, rule := range existing {
		allowsOrigin := false
		for _, o := range rule.AllowedOrigins {
			if o == origin || o == "*" {
				allowsOrigin = true
				break
			}
		}
		if !allowsOrigin {
			continue
		}
		for _, m := range rule.AllowedMethods {
			if m == "GET" {
				return existing, false
			}
		}
	}

	merged := make([]contracts.CORSRule, len(existing), len(existing)+1)
	copy(merged, existing)
	merged = append(merged, contracts.CORSRule{
		ID:             "ponta-drive-preview",
		AllowedOrigins: []string{origin},
		AllowedMethods: []string{"GET", "HEAD"},
		AllowedHeaders: []string{"*"},
		ExposeHeaders:  []string{"ETag", "Content-Range", "Content-Length"},
		MaxAgeSeconds:  3000,
	})
	return merged, true
}