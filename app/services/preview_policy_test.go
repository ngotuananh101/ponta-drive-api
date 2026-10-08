package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"ponta_drive/app/contracts"
)

func TestClassifyPreviewKind(t *testing.T) {
	cases := []struct {
		name string
		mime string
		ext  string
		want PreviewKind
	}{
		{"image by mime", "image/png", "png", KindMedia},
		{"video by mime", "video/mp4", "mp4", KindMedia},
		{"audio by mime", "audio/mpeg", "mp3", KindMedia},
		{"image by extension when mime empty", "", "png", KindMedia},
		{"video by extension when mime generic", "application/octet-stream", "mp4", KindMedia},
		{"pdf is fetch-based", "application/pdf", "pdf", KindFetchBased},
		{"office is fetch-based", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx", KindFetchBased},
		{"text is fetch-based", "text/plain", "txt", KindFetchBased},
		{"unknown is unsupported", "application/x-msdownload", "exe", KindUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ClassifyPreviewKind(tc.mime, tc.ext))
		})
	}
}

func TestDecidePreviewStrategy(t *testing.T) {
	// Media: always direct (no CORS needed), regardless of size.
	s, reason := DecidePreviewStrategy(KindMedia, 10, false)
	assert.Equal(t, StrategyDirect, s)
	assert.Empty(t, reason)

	// Fetch-based, within the cap: proxy.
	s, _ = DecidePreviewStrategy(KindFetchBased, MaxPreviewProxyBytes, false)
	assert.Equal(t, StrategyProxy, s)

	// Fetch-based, one byte over the cap, no public URL: fallback.
	s, reason = DecidePreviewStrategy(KindFetchBased, MaxPreviewProxyBytes+1, false)
	assert.Equal(t, StrategyFallback, s)
	assert.Equal(t, "too_large", reason)

	// Fetch-based, over the cap, but a reachable public URL exists: direct.
	s, _ = DecidePreviewStrategy(KindFetchBased, MaxPreviewProxyBytes+1, true)
	assert.Equal(t, StrategyDirect, s)

	// Fetch-based, within the cap, reachable public URL: still proxy (safer).
	s, _ = DecidePreviewStrategy(KindFetchBased, 10, true)
	assert.Equal(t, StrategyProxy, s)

	// Unsupported: fallback.
	s, reason = DecidePreviewStrategy(KindUnsupported, 10, true)
	assert.Equal(t, StrategyFallback, s)
	assert.Equal(t, "unsupported", reason)
}

func TestParseRangeHeader(t *testing.T) {
	// No header: whole object.
	offset, length, err := ParseRangeHeader("", 100)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, int64(100), length)

	// bytes=0- -> to end.
	offset, length, err = ParseRangeHeader("bytes=0-", 100)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, int64(100), length)

	// bytes=10-19 -> 10 bytes.
	offset, length, err = ParseRangeHeader("bytes=10-19", 100)
	assert.NoError(t, err)
	assert.Equal(t, int64(10), offset)
	assert.Equal(t, int64(10), length)

	// bytes=90- -> clamped to the end.
	offset, length, err = ParseRangeHeader("bytes=90-", 100)
	assert.NoError(t, err)
	assert.Equal(t, int64(90), offset)
	assert.Equal(t, int64(10), length)

	// Start past the end: unsatisfiable.
	_, _, err = ParseRangeHeader("bytes=200-300", 100)
	assert.Error(t, err)

	// Malformed.
	_, _, err = ParseRangeHeader("items=0-5", 100)
	assert.Error(t, err)

	// Multi-range is not supported.
	_, _, err = ParseRangeHeader("bytes=0-5,10-15", 100)
	assert.Error(t, err)
}

func TestMergeCorsRules(t *testing.T) {
	// No existing rules -> one added rule.
	merged, changed := MergeCorsRules(nil, "https://app.example")
	assert.True(t, changed)
	assert.Len(t, merged, 1)
	assert.Equal(t, []string{"https://app.example"}, merged[0].AllowedOrigins)
	assert.Equal(t, []string{"GET", "HEAD"}, merged[0].AllowedMethods)

	// Existing unrelated rule is preserved.
	existing := []contracts.CORSRule{{ID: "other", AllowedOrigins: []string{"https://other"}, AllowedMethods: []string{"GET"}}}
	merged, changed = MergeCorsRules(existing, "https://app.example")
	assert.True(t, changed)
	assert.Len(t, merged, 2)
	assert.Equal(t, "other", merged[0].ID)

	// Our rule already present -> idempotent no-op.
	ours := []contracts.CORSRule{{AllowedOrigins: []string{"https://app.example"}, AllowedMethods: []string{"GET", "HEAD"}}}
	merged, changed = MergeCorsRules(ours, "https://app.example")
	assert.False(t, changed)
	assert.Len(t, merged, 1)
}