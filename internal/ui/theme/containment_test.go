package theme_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// banned are the ways a color can be named outside the theme package. Every one
// of them puts a color somewhere it cannot be themed.
var banned = []string{
	"lipgloss.Color(",
	"AdaptiveColor",
	"lipgloss.LightDark",
}

// namesColor reports whether src mentions a color in any of the banned ways.
func namesColor(src []byte) bool {
	for _, b := range banned {
		if strings.Contains(string(src), b) {
			return true
		}
	}
	return false
}

// No file under internal/ui may name a color. The theme package is the only
// place a color literal is allowed to appear. This is what makes "all colors in
// one place" a property of the codebase rather than a state it was briefly in.
func TestNoColorsOutsideThemePackage(t *testing.T) {
	root := ".." // internal/ui
	var offenders []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "theme" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if namesColor(src) {
			offenders = append(offenders, filepath.ToSlash(rel))
		}
		return nil
	})
	require.NoError(t, err)

	assert.Empty(t, offenders, "colors must live in internal/ui/theme only")
}
