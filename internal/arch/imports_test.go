// Package arch holds tests about the shape of the codebase rather than its
// behaviour: boundaries that are easy to state and easy to erode.
package arch_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const storePkg = "github.com/sorokin-vladimir/tele/internal/store"

// A client renders from projections (#194) and asks the owner everything else
// (#278); reaching into the persistence package is how that boundary erodes.
// There used to be an allowlist of files still permitted the import while their
// issues were open. #278 removed the last one, so there are no exceptions left.
//
// Direct imports are what is checked. The store still arrives transitively,
// through the protocol types the client takes from core and tg; moving those
// into a package of their own belongs with the socket (#209).
func TestUIDoesNotImportStore(t *testing.T) {
	fset := token.NewFileSet()
	for _, path := range goFilesUnder(t, filepath.Join("..", "ui")) {
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		require.NoError(t, err)
		for _, imp := range f.Imports {
			if strings.Trim(imp.Path.Value, `"`) == storePkg {
				assert.Fail(t, "the UI must not read domain state",
					"%s imports internal/store; take it from a projection or an owner query instead (#194, #278)", path)
			}
		}
	}
}

// goFilesUnder collects every non-test Go source under root, including
// subpackages: components and screens are as much a client as root is.
func goFilesUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		out = append(out, path)
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, out, "no UI sources found — the walk root is wrong")
	return out
}
