package mail

import (
	"bytes"
	"html/template"
	"path/filepath"

	"github.com/goravel/framework/errors"
	"github.com/goravel/framework/support"
)

type LayoutEngine struct {
	viewsPath string
}

func NewLayoutEngine(viewsPath string) *LayoutEngine {
	return &LayoutEngine{
		viewsPath: viewsPath,
	}
}

func (e *LayoutEngine) Render(viewFile string, data any) (string, error) {
	layoutPath := filepath.Join(support.RelativePath, e.viewsPath, "layout.html")
	contentPath := filepath.Join(support.RelativePath, e.viewsPath, viewFile)

	tmpl, err := template.ParseFiles(layoutPath, contentPath)
	if err != nil {
		return "", errors.MailTemplateParseFailed.Args(viewFile, err)
	}

	var buf bytes.Buffer
	// layout.html defines the main document which invokes {{ template "content" . }}
	if err := tmpl.ExecuteTemplate(&buf, "layout.html", data); err != nil {
		return "", errors.MailTemplateExecutionFailed.Args(viewFile, err)
	}

	return buf.String(), nil
}
