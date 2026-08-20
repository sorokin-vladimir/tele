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

// A theme may paint the canvas, and the colour cannot be applied by wrapping the
// composed view: a line holding a coloured run carries an SGR reset in the
// middle of it, and anything wrapped around that loses the background from the
// reset onward. So the canvas is baked into every cell at the point the cell is
// created.
//
// That makes every emitter of cells a place the canvas can be forgotten, and a
// forgotten one is a hole visible only to whoever happens to be looking at the
// right screen state. There are two kinds of emitter, and this refuses the bare
// form of both, so the rule is enforced by the build rather than remembered:
//
//   - a style, which must come from theme.NewStyle;
//   - a run of spaces padding a row out to a width, which must come from
//     theme.Pad or theme.PadTo.
//
// Tests are exempt: a style built to assert something never reaches a screen.
//
// Not every bare form is wrong. Spaces that end up inside a string a style later
// renders are already painted by that style, and painting them again would put
// an escape in the middle of text lipgloss is about to measure. Such a site opts
// out with a canvasOK comment on the line or the line above, which says why —
// the point of the rule is that every exception is written down, not that there
// are none.

// guardRoot is the tree the rule covers, relative to this file.
const guardRoot = "."

// guardExempt is the package the rule cannot apply to, because it is where the
// replacements are defined.
var guardExempt = filepath.Join("theme")

// canvasOK marks a site the rule does not apply to. It must be followed by a
// reason: an exception nobody had to justify is the discipline this replaces.
const canvasOK = "canvas:ok"

func TestCanvas_NoBareStyleConstructor(t *testing.T) {
	found := scanGuarded(t, func(n ast.Node) string {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return ""
		}
		if isSelector(call.Fun, "lipgloss", "NewStyle") {
			return "lipgloss.NewStyle(): use theme.NewStyle(), which carries the canvas"
		}
		return ""
	})
	require.Empty(t, found, "a style built outside theme.NewStyle leaves a hole in the canvas")
}

func TestCanvas_NoBareSpacePadding(t *testing.T) {
	found := scanGuarded(t, func(n ast.Node) string {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return ""
		}
		if !isSelector(call.Fun, "strings", "Repeat") || len(call.Args) == 0 {
			return ""
		}
		if !isSpaceLiteral(call.Args[0]) {
			return "" // repeating a border rune is not padding
		}
		return `strings.Repeat(" ", n): use theme.Pad(n) or theme.PadTo(w, width)`
	})
	require.Empty(t, found, "padding emitted as bare spaces leaves a hole in the canvas")
}

// The third emitter, and the one #227 was: text put into a line as a bare string
// literal, never handed to a style at all. It is refused where it is glued
// straight onto a rendered run, because that shape is a line being composed and
// a literal in the middle of one reaches a cell carrying nothing.
//
// This is a detector for the common shape, not a proof, and the difference
// matters to whoever sees it green next. Of the seven sites #227 found it
// catches four. The sender name was `" " + senderStyled + " "`, where the
// .Render is two lines above and not in the expression — reachable with
// function-local dataflow, which this does not do. The media placeholder is
// `return "📷 photo"`: no concatenation, no .Render, and a hole only three
// frames up the stack, at a consumer that cannot see the literal. The album
// badge is literals with no .Render anywhere near them.
//
// Those last two are not reachable by any purely syntactic rule, because the
// literal is legitimate where it is written. Broadening this to every string
// literal in a concatenation under internal/ui is 136 sites across some sixty
// files, most of them not screen text at all — a rule that does not close the
// class, only teaches everyone to write canvas:ok. What actually closes it is
// the cell scan in canvas_holes_test.go, over a fixture built from the domain's
// own enumerations. This rule is here because it is cheap and reports on the
// right line.
func TestCanvas_NoLiteralGluedToRenderedRun(t *testing.T) {
	found := scanGuarded(t, func(n ast.Node) string {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok || bin.Op != token.ADD {
			return ""
		}
		var literal, rendered bool
		for _, op := range addOperands(bin) {
			if isStringLiteral(op) {
				literal = true
			}
			if hasRenderCall(op) {
				rendered = true
			}
		}
		if !literal || !rendered {
			return ""
		}
		return `a literal glued to a rendered run: render it with the run, or use theme.Pad`
	})
	require.Empty(t, found, "a literal beside a rendered run reaches a cell with no canvas on it")
}

// addOperands flattens a chain of + into its operands. Go parses a + b + c as
// (a + b) + c, so the operands of one composed line are spread over two nodes
// and a rule reading either alone sees half of it.
func addOperands(expr ast.Expr) []ast.Expr {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != token.ADD {
		return []ast.Expr{expr}
	}
	return append(addOperands(bin.X), addOperands(bin.Y)...)
}

// hasRenderCall reports whether expr calls a Render method anywhere inside it.
// Matching on the method name rather than on a type keeps this a parser and not
// a type checker; lipgloss is the only thing under internal/ui with a Render.
func hasRenderCall(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Render" {
			found = true
			return false
		}
		return true
	})
	return found
}

// isStringLiteral reports whether expr is a string literal with something in it.
// An empty one emits no cell, so it has nothing to leave unpainted.
func isStringLiteral(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	s, err := strconv.Unquote(lit.Value)
	return err == nil && s != ""
}

// scanGuarded parses every non-test Go file under the guarded tree and reports
// what report names, as "file:line: message".
//
// Findings are deduplicated by that string, which is what lets a rule fire on
// every node of a nested expression without saying the same thing twice: the
// operands of `a + b + c` are visited as two chains that start at the same
// token, so both land on one position.
func scanGuarded(t *testing.T, report func(ast.Node) string) []string {
	t.Helper()

	var found []string
	seen := map[string]bool{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(guardRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != guardRoot && filepath.Base(path) == guardExempt {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		exempt := exemptLines(fset, file)
		ast.Inspect(file, func(n ast.Node) bool {
			if n == nil {
				return false
			}
			pos := fset.Position(n.Pos())
			if exempt[pos.Line] || exempt[pos.Line-1] {
				return true
			}
			msg := report(n)
			if msg == "" {
				return true
			}
			if entry := pos.String() + ": " + msg; !seen[entry] {
				seen[entry] = true
				found = append(found, entry)
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	return found
}

// exemptLines returns the lines carrying a canvasOK marker with a reason after
// it. A bare marker is not an exemption: it exempts nothing and shows up as the
// violation it was meant to silence, which is the only way to keep the reason
// from becoming optional.
// The marker holds for the whole comment group it appears in, so a reason may
// run to as many lines as it takes without the first line silently being the
// only one that counts.
func exemptLines(fset *token.FileSet, file *ast.File) map[int]bool {
	lines := map[int]bool{}
	for _, group := range file.Comments {
		if !hasReason(group) {
			continue
		}
		for l := fset.Position(group.Pos()).Line; l <= fset.Position(group.End()).Line; l++ {
			lines[l] = true
		}
	}
	return lines
}

// hasReason reports whether a comment group carries the marker followed by
// something. A bare marker exempts nothing and shows up as the violation it was
// meant to silence, which is the only way to keep the reason from being
// optional.
func hasReason(group *ast.CommentGroup) bool {
	for _, c := range group.List {
		i := strings.Index(c.Text, canvasOK)
		if i < 0 {
			continue
		}
		if strings.TrimSpace(c.Text[i+len(canvasOK):]) != "" {
			return true
		}
	}
	return false
}

// isSelector reports whether expr is the call pkg.name, matching on the package
// identifier as written rather than on the import path: a file that aliases the
// import is a file that has been thought about.
func isSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg
}

// isSpaceLiteral reports whether expr is a string literal of nothing but spaces.
// A single space is the padding case; anything else is drawing something.
func isSpaceLiteral(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil || s == "" {
		return false
	}
	return strings.TrimLeft(s, " ") == ""
}
