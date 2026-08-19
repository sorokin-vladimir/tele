package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// configFileMode is what a config file is created with when the original's mode
// cannot be read. It holds an API hash, so it is not world-readable.
const configFileMode os.FileMode = 0600

// indentStep is what a nested key is indented by when one has to be written.
// It matches what tele writes on first launch and every documented example.
const indentStep = 2

// editFile changes one key in the YAML file at path and leaves every other byte
// of it alone: comments, blank lines, key order, quoting, indentation, and any
// key this binary knows nothing about (ADR 0009).
//
// A nil value deletes the key. Absence is how a setting returns to its default,
// so the file does not grow into an exhaustive dump of every default.
//
// The node tree is used to find the line to change, and the change is made to
// the text. Re-encoding the tree was tried first and is not good enough: yaml.v3
// drops every blank line and re-spaces every inline comment, so the first
// setting anyone changed would reformat the whole file, including the parts the
// app itself wrote to explain the file. Locating with the parser and editing the
// text keeps both promises at once - the app understands the file, and the file
// stays the person's.
//
// The file is read here rather than held from when the overlay opened: this is a
// read-modify-write against a file somebody may also have in an editor, and the
// window worth narrowing is the one between reading and writing.
func editFile(path, key string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		// Never write a file that could not be read. Whatever is in there is
		// the person's, and a parse failure is the case the app understands
		// least.
		return fmt.Errorf("%s: %w", path, err)
	}

	src := newSource(raw)
	segments := strings.Split(key, ".")
	root := documentRoot(&doc)
	if root == nil && value == nil {
		return nil // Nothing in the file, nothing to delete.
	}
	if root == nil {
		root = &yaml.Node{Kind: yaml.MappingNode}
	}
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: the file's top level is not a mapping of keys", path)
	}

	changed := false
	if value == nil {
		changed = src.delete(root, segments)
	} else {
		changed, err = src.set(root, segments, value)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	if !changed {
		// Resetting a setting the file never named, most often. The file is
		// already saying what was asked for, and rewriting it would be a change
		// nobody made.
		return nil
	}

	out := src.text()
	if err := verify(out, segments, value); err != nil {
		return fmt.Errorf("%s: refusing to write: %w", path, err)
	}
	return writeAtomic(path, []byte(out), fileMode(path))
}

// documentRoot returns the mapping every key hangs off, or nil for a file that
// holds no keys at all.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return nil
	}
	return root
}

// source is the file as lines, edited in place.
type source struct {
	lines []string
	// endsWithNewline records whether the file ended in one, so that writing a
	// setting does not silently add or remove the last byte.
	endsWithNewline bool
}

func newSource(raw []byte) *source {
	text := string(raw)
	s := &source{endsWithNewline: strings.HasSuffix(text, "\n")}
	if text == "" {
		return s
	}
	s.lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	return s
}

func (s *source) text() string {
	out := strings.Join(s.lines, "\n")
	if s.endsWithNewline || out == "" {
		out += "\n"
	}
	return out
}

// set writes value at the given path, following the mapping nodes as far as the
// file goes and writing the rest of the path as new lines. It reports whether
// anything changed.
func (s *source) set(node *yaml.Node, segments []string, value any) (bool, error) {
	for i, seg := range segments {
		keyNode, valueNode := findKey(node, seg)
		last := i == len(segments)-1

		if keyNode == nil {
			s.insert(node, segments[:i], segments[i:], value, indentUnder(node, i))
			return true, nil
		}
		if last {
			return s.replace(keyNode, valueNode, value), nil
		}
		if valueNode.Kind != yaml.MappingNode {
			return false, fmt.Errorf("%s is not a section in this file", strings.Join(segments[:i+1], "."))
		}
		node = valueNode
	}
	return false, nil
}

// replace rewrites the value beside a key, keeping whatever else is on the line
// - most often a comment somebody wrote about the setting, which describes the
// setting and not the number.
func (s *source) replace(keyNode, valueNode *yaml.Node, value any) bool {
	rendered := render(value, valueNode)

	// A value that spans lines - a theme written as a dark/light pair, say -
	// becomes a single line when it is set to a scalar, so the whole block goes.
	if valueNode.Kind != yaml.ScalarNode {
		start := keyNode.Line - 1
		end := s.blockEnd(start, indentOf(s.lines[start]))
		s.lines = slices.Replace(s.lines, start, end,
			s.lines[start][:keyNode.Column-1]+keyNode.Value+": "+rendered)
		return true
	}

	i := valueNode.Line - 1
	line := s.lines[i]
	head := line[:valueNode.Column-1]
	tail := ""
	if comment := lineComment(keyNode, valueNode); comment != "" {
		if at := strings.LastIndex(line, comment); at >= 0 {
			gap := at
			for gap > 0 && (line[gap-1] == ' ' || line[gap-1] == '\t') {
				gap--
			}
			// The run of spaces before the comment is somebody's alignment.
			tail = line[gap:]
		}
	}
	replaced := head + rendered + tail
	if replaced == line {
		return false
	}
	s.lines[i] = replaced
	return true
}

// insert writes a key that is not in the file yet, and any section on the way to
// it, at the place the registry says it belongs - so a key the app adds lands
// where a person reading the file would look for it rather than at the bottom.
func (s *source) insert(parent *yaml.Node, parentPath, rest []string, value any, indent int) {
	block := make([]string, 0, len(rest))
	for i, seg := range rest {
		pad := strings.Repeat(" ", indent+i*indentStep)
		if i == len(rest)-1 {
			block = append(block, pad+seg+": "+render(value, nil))
			continue
		}
		block = append(block, pad+seg+":")
	}
	s.lines = slices.Insert(s.lines, s.insertionLine(parent, parentPath, rest[0]), block...)
}

// insertionLine finds the line a new key goes on: after the last sibling the
// registry declares before it, and otherwise before the first one it declares
// after it.
//
// Comment lines above a key are that key's, so an insertion steps back over them
// rather than landing between a comment and what it describes. Blank lines above
// those are the separation between one thing and the next, and are stepped back
// over too, so a new key joins the block it belongs to instead of starting a new
// one.
func (s *source) insertionLine(parent *yaml.Node, parentPath []string, key string) int {
	order := siblingOrder(parentPath)
	rank := slices.Index(order, key)

	if rank >= 0 {
		var after *yaml.Node
		var before *yaml.Node
		for i := 0; i+1 < len(parent.Content); i += 2 {
			switch r := slices.Index(order, parent.Content[i].Value); {
			case r < 0:
				// Not the app's key. Somebody else put it there and it keeps
				// its place.
			case r < rank:
				after = parent.Content[i]
			case r > rank && before == nil:
				before = parent.Content[i]
			}
		}
		if after != nil {
			start := after.Line - 1
			return s.blockEnd(start, indentOf(s.lines[start]))
		}
		if before != nil {
			return s.stepBackOverComments(before.Line - 1)
		}
	}
	if len(parent.Content) == 0 {
		return len(s.lines)
	}
	first := parent.Content[0]
	return s.blockEnd(first.Line-1, indentOf(s.lines[first.Line-1]))
}

// delete removes a key, and then any section the removal left empty - an empty
// section is a leftover of an edit rather than something anybody wrote. A key's
// comments are left where they are: they are somebody's writing, and "reset this
// setting" did not ask for them to go.
func (s *source) delete(node *yaml.Node, segments []string) bool {
	keyNode, valueNode := findKey(node, segments[0])
	if keyNode == nil {
		return false
	}
	if len(segments) > 1 {
		if valueNode.Kind != yaml.MappingNode {
			return false
		}
		if !s.delete(valueNode, segments[1:]) {
			return false
		}
		// The child was the section's last key if the section held one key.
		if len(valueNode.Content) > 2 || hasComment(keyNode) || hasComment(valueNode) {
			return true
		}
	}
	start := keyNode.Line - 1
	s.lines = slices.Delete(s.lines, start, s.blockEnd(start, indentOf(s.lines[start])))
	s.collapseBlankRun(start)
	return true
}

// collapseBlankRun drops one blank line where a deletion has left two. Blank
// lines separate one thing from the next; two of them are what is left when the
// thing between them goes, and they would pile up one reset at a time.
func (s *source) collapseBlankRun(at int) {
	if at <= 0 || at >= len(s.lines) {
		return
	}
	if isBlank(s.lines[at-1]) && isBlank(s.lines[at]) {
		s.lines = slices.Delete(s.lines, at, at+1)
	}
}

// blockEnd returns the line index just past what belongs to a key that starts at
// start and sits at the given indent.
//
// Blank lines and comment lines never end a block and never extend one: a
// comment after the last value of a section belongs to whatever comes next, and
// a blank line is separation. Only content indented deeper than the key extends
// it.
func (s *source) blockEnd(start, indent int) int {
	last := start
	for i := start + 1; i < len(s.lines); i++ {
		line := s.lines[i]
		if isBlank(line) || isComment(line) {
			continue
		}
		if indentOf(line) <= indent {
			break
		}
		last = i
	}
	return last + 1
}

// stepBackOverComments walks up from a key over the comment lines that describe
// it, and then over the blank lines above those.
func (s *source) stepBackOverComments(at int) int {
	for at > 0 && isComment(s.lines[at-1]) {
		at--
	}
	for at > 0 && isBlank(s.lines[at-1]) {
		at--
	}
	return at
}

// indentUnder is how deep a new key under a mapping should sit: level with the
// keys already there, and otherwise one step in per level of nesting, because
// there is nothing in the file to copy.
func indentUnder(node *yaml.Node, depth int) int {
	if len(node.Content) > 0 {
		return node.Content[0].Column - 1
	}
	return depth * indentStep
}

// siblingOrder returns the keys that live directly under the given path, in
// registry order. Exclusions are included: keybindings is not a setting, but it
// is a key of this file, and something inserted beside it should know where it
// sits.
func siblingOrder(parent []string) []string {
	prefix := ""
	if len(parent) > 0 {
		prefix = strings.Join(parent, ".") + "."
	}
	var order []string
	for _, key := range allKeys() {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(key, prefix), ".")
		if name != "" && !slices.Contains(order, name) {
			order = append(order, name)
		}
	}
	return order
}

// allKeys is every key of this file the app has an opinion about, declared and
// excluded alike, in the order they are declared.
func allKeys() []string {
	keys := make([]string, 0, len(registry)+len(exclusions))
	for _, e := range registry {
		keys = append(keys, e.Key)
	}
	for _, x := range exclusions {
		keys = append(keys, x.Key)
	}
	return keys
}

// findKey returns the key and value nodes for a key inside a mapping.
func findKey(node *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i], node.Content[i+1]
		}
	}
	return nil, nil
}

func lineComment(keyNode, valueNode *yaml.Node) string {
	if valueNode != nil && valueNode.LineComment != "" {
		return valueNode.LineComment
	}
	if keyNode != nil {
		return keyNode.LineComment
	}
	return ""
}

func hasComment(n *yaml.Node) bool {
	return n != nil && (n.HeadComment != "" || n.LineComment != "" || n.FootComment != "")
}

func indentOf(line string) int { return len(line) - len(strings.TrimLeft(line, " \t")) }

func isBlank(line string) bool { return strings.TrimSpace(line) == "" }

func isComment(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "#") }

// render writes a Go value the way YAML would, keeping the quoting of the value
// it replaces. Turning "deadbeef" into deadbeef is a change nobody asked for in
// a file people read.
func render(value any, like *yaml.Node) string {
	var n yaml.Node
	if err := n.Encode(value); err != nil {
		return fmt.Sprint(value)
	}
	if like != nil && like.Kind == yaml.ScalarNode && like.Tag == n.Tag &&
		(like.Style == yaml.DoubleQuotedStyle || like.Style == yaml.SingleQuotedStyle) {
		n.Style = like.Style
	}
	out, err := yaml.Marshal(&n)
	if err != nil {
		return fmt.Sprint(value)
	}
	return strings.TrimRight(string(out), "\n")
}

// verify re-reads the edited text before any of it reaches the file. Editing
// text that a parser located is only safe if the result is checked by the same
// parser, so this is where a splice that produced something other than what was
// asked for stops.
func verify(text string, segments []string, want any) error {
	var got any
	if err := yaml.Unmarshal([]byte(text), &got); err != nil {
		return err
	}
	for _, seg := range segments {
		m, ok := got.(map[string]any)
		if !ok {
			got = nil
			break
		}
		got = m[seg]
	}
	key := strings.Join(segments, ".")
	if want == nil {
		if got != nil {
			return fmt.Errorf("%s is still set", key)
		}
		return nil
	}
	if !equalValue(got, want) {
		return fmt.Errorf("%s reads back as %#v, not %#v", key, got, want)
	}
	return nil
}

// equalValue compares what YAML read back with what was asked for, across the
// integer widths a number arrives in.
func equalValue(got, want any) bool {
	if reflect.DeepEqual(got, want) {
		return true
	}
	g, gok := toInt64(got)
	w, wok := toInt64(want)
	return gok && wok && g == w
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		return int64(n), true
	case uint64:
		return int64(n), true
	case float64:
		i := int64(n)
		return i, float64(i) == n
	}
	return 0, false
}

func fileMode(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return configFileMode
	}
	return info.Mode().Perm()
}

// writeAtomic replaces the file's contents in one step. A crash halfway leaves
// the file that was there, never half of the file that was going to be.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	// Harmless once the rename has happened; the point is the paths that leave
	// through an error return.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
