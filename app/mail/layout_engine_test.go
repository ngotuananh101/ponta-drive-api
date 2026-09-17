package mail

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/goravel/framework/errors"
	"github.com/goravel/framework/support"
)

// backendRoot resolves to the project backend directory from this test file
// (located at backend/app/mail/layout_engine_test.go).
func backendRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

func TestLayoutEngine_Render(t *testing.T) {
	original := support.RelativePath
	support.RelativePath = backendRoot(t)
	defer func() { support.RelativePath = original }()

	engine := NewLayoutEngine("resources/views/mail")

	data := map[string]any{
		"Subject":        "Đặt lại mật khẩu Ponta Drive",
		"Year":           2026,
		"Name":           "Nguyen Van A",
		"Email":          "user@example.com",
		"ResetUrl":       "https://ponta-drive.test/reset-password?token=abc123",
		"ExpireMinutes":  60,
	}

	out, err := engine.Render("reset_password.html", data)
	assert.NoError(t, err)
	assert.NotEmpty(t, out)

	// Layout wrappers are rendered.
	assert.Contains(t, out, "<!DOCTYPE html>")
	assert.Contains(t, out, "Ponta Drive")
	assert.Contains(t, out, "&copy; 2026 Ponta Drive")
	assert.Contains(t, out, "<title>Đặt lại mật khẩu Ponta Drive</title>")

	// Content block is rendered inside the layout.
	assert.Contains(t, out, "Xin chào <strong>Nguyen Van A</strong>")
	assert.Contains(t, out, "user@example.com")
	assert.Contains(t, out, "Đặt lại mật khẩu")
	assert.Contains(t, out, "https://ponta-drive.test/reset-password?token=abc123")
	assert.Contains(t, out, "60 phút")

	// The content slot placeholder is resolved, not left literal.
	assert.NotContains(t, out, `{{ template "content" . }}`)
}

func TestLayoutEngine_RenderTemplateNotFound(t *testing.T) {
	original := support.RelativePath
	support.RelativePath = backendRoot(t)
	defer func() { support.RelativePath = original }()

	engine := NewLayoutEngine("resources/views/mail")

	_, err := engine.Render("does_not_exist.html", nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errors.MailTemplateParseFailed)
}
