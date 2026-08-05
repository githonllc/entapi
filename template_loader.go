package entdomain

import (
	"embed"
	"fmt"
	"path"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// loadTemplate reads a named template from the embedded filesystem.
// The name should not include the "templates/" prefix or ".tmpl" suffix.
// Paths use path.Join, not filepath.Join: embedded FS paths are slash-separated
// on every platform, so a separator-aware join would miss on Windows.
func loadTemplate(name string) (string, error) {
	filename := path.Join("templates", name+".tmpl")
	content, err := templateFS.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to load template %s: %w", filename, err)
	}
	return string(content), nil
}

// mustLoadTemplate loads a named template and panics on failure.
// Use this for templates that are required at package init time.
func mustLoadTemplate(name string) string {
	content, err := loadTemplate(name)
	if err != nil {
		panic(err)
	}
	return content
}
