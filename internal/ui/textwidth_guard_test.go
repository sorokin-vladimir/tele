package ui_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type bannedWidthCall struct {
	path string
	name string
	use  string
}

// Calls that measure, wrap or slice by a rule other than the terminal's. They
// are matched by import path, so an aliased import cannot hide one.
var bannedWidthCalls = []bannedWidthCall{
	{"charm.land/lipgloss/v2", "Width", "textwidth.Width"},
	{"charm.land/lipgloss/v2", "Wrap", "textwidth.Wrap"},
	{"github.com/charmbracelet/x/ansi", "StringWidth", "textwidth.Width"},
	{"github.com/charmbracelet/x/ansi", "Truncate", "textwidth.Truncate"},
	{"github.com/charmbracelet/x/ansi", "TruncateLeft", "textwidth.TruncateLeft"},
	{"github.com/charmbracelet/x/ansi", "Cut", "textwidth.Cut"},
	{"github.com/charmbracelet/x/ansi", "Wrap", "textwidth.Wrap"},
	{"github.com/charmbracelet/x/ansi", "Wordwrap", "textwidth.Wordwrap"},
	{"github.com/mattn/go-runewidth", "StringWidth", "textwidth.Width"},
}

// Every direct width measurement, wrap and slice in the UI goes through
// internal/ui/textwidth, which follows go-runewidth. A call to one of the
// library functions directly measures by a different rule (Unicode emoji
// presentation, for lipgloss and xansi), and a row measured one way and padded
// the other ends one cell off its border.
//
// This refuses those calls. Tests are exempt, and so is textwidth itself, which
// is where the rule is defined. Method calls such as Style.Width are not width
// functions; they are left to their callers.
func TestTextWidth_NoDirectMeasurement(t *testing.T) {
	var found []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == filepath.Join("textwidth") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		imports := localImportNames(file)
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			importPath := imports[ident.Name]
			for _, banned := range bannedWidthCalls {
				if importPath == banned.path && sel.Sel.Name == banned.name {
					found = append(found, fset.Position(call.Pos()).String()+": "+banned.path+"."+banned.name+"(): use "+banned.use)
				}
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, found, "a direct call measures by a rule the terminal does not follow")
}

// localImportNames maps the name an import is written as to its path, so an
// aliased import cannot hide a call.
func localImportNames(file *ast.File) map[string]string {
	names := make(map[string]string, len(file.Imports))
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := defaultImportName(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "" {
			names[name] = path
		}
	}
	return names
}

func defaultImportName(path string) string {
	switch path {
	case "charm.land/lipgloss/v2":
		return "lipgloss"
	case "github.com/charmbracelet/x/ansi":
		return "ansi"
	case "github.com/mattn/go-runewidth":
		return "runewidth"
	default:
		return ""
	}
}
