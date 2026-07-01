package screens

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// FileSelectedMsg is emitted when the user selects a file (not a directory).
type FileSelectedMsg struct{ Path string }

// CloseFilePickerMsg is emitted when the user closes the picker without selecting.
type CloseFilePickerMsg struct{}

const filePickerWidth = 60
const filePickerMaxRows = 12

// FilePickerModel is a file-browser overlay: it lists a directory (folders above
// files), filters by typed text, descends/ascends, and resolves a pasted path.
// It is type-agnostic — selecting a file emits FileSelectedMsg and the caller
// decides what to do with it (#106 photo, #107 video, #129 document).
type FilePickerModel struct {
	dir     string
	all     []os.DirEntry // raw entries (dirs first), excluding ".."
	entries []os.DirEntry // filtered view, with a synthetic ".." prepended
	filter  string
	list    *components.ListView
	width   int
	height  int
	keyMap  keys.KeyMap
}

func NewFilePickerModel(startDir string, width, height int, km keys.KeyMap) *FilePickerModel {
	if startDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			startDir = home
		} else {
			startDir = "."
		}
	}
	m := &FilePickerModel{dir: startDir, width: width, height: height, keyMap: km, list: components.NewListView(false)}
	m.readDir()
	return m
}

func (m *FilePickerModel) Dir() string            { return m.dir }
func (m *FilePickerModel) Cursor() int            { return m.list.Cursor() }
func (m *FilePickerModel) Entries() []os.DirEntry { return m.entries }

// CurrentName returns the name of the entry under the cursor ("" if none).
func (m *FilePickerModel) CurrentName() string {
	c := m.list.Cursor()
	if c < 0 || c >= len(m.entries) {
		return ""
	}
	return m.entries[c].Name()
}

// dotDot is a synthetic directory entry for the parent directory.
type dotDot struct{}

func (dotDot) Name() string               { return ".." }
func (dotDot) IsDir() bool                { return true }
func (dotDot) Type() os.FileMode          { return os.ModeDir }
func (dotDot) Info() (os.FileInfo, error) { return nil, nil }

func (m *FilePickerModel) readDir() {
	ents, err := os.ReadDir(m.dir)
	if err != nil {
		ents = nil
	}
	sort.SliceStable(ents, func(i, j int) bool {
		di, dj := ents[i].IsDir(), ents[j].IsDir()
		if di != dj {
			return di // directories first
		}
		return strings.ToLower(ents[i].Name()) < strings.ToLower(ents[j].Name())
	})
	m.all = ents
	m.filter = ""
	m.applyFilter()
	m.list.SetCursor(0) // reset to top when entering a directory
}

func (m *FilePickerModel) applyFilter() {
	q := strings.ToLower(m.filter)
	out := []os.DirEntry{}
	// ".." is shown only when not at the filesystem root.
	if parent := filepath.Dir(m.dir); parent != m.dir {
		if q == "" || strings.Contains("..", q) {
			out = append(out, dotDot{})
		}
	}
	for _, e := range m.all {
		if q == "" || strings.Contains(strings.ToLower(e.Name()), q) {
			out = append(out, e)
		}
	}
	m.entries = out
	m.list.SetCount(len(m.entries))
}

func (m *FilePickerModel) ascend() {
	m.dir = filepath.Dir(m.dir)
	m.readDir()
}

func (m *FilePickerModel) descend(name string) {
	m.dir = filepath.Join(m.dir, name)
	m.readDir()
}

func (m *FilePickerModel) Update(msg tea.Msg) (*FilePickerModel, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		return m, m.resolvePath(strings.TrimSpace(paste.Content))
	}
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch kp.Code {
	case tea.KeyEsc:
		return m, func() tea.Msg { return CloseFilePickerMsg{} }
	case tea.KeyEnter:
		return m.confirm()
	case tea.KeyBackspace:
		if m.filter != "" {
			r := []rune(m.filter)
			m.filter = string(r[:len(r)-1])
			m.applyFilter()
			return m, nil
		}
		m.ascend()
		return m, nil
	case tea.KeyDown:
		m.moveDown()
		return m, nil
	case tea.KeyUp:
		m.moveUp()
		return m, nil
	}
	switch keys.NormalizeKey(kp.String()) {
	case "ctrl+j":
		m.moveDown()
	case "ctrl+k":
		m.moveUp()
	case "-":
		m.ascend()
	default:
		if kp.Text != "" {
			m.filter += kp.Text
			m.applyFilter()
		}
	}
	return m, nil
}

func (m *FilePickerModel) moveDown() { m.list.MoveDown() }
func (m *FilePickerModel) moveUp()   { m.list.MoveUp() }

func (m *FilePickerModel) confirm() (*FilePickerModel, tea.Cmd) {
	name := m.CurrentName()
	if name == "" {
		return m, nil
	}
	if name == ".." {
		m.ascend()
		return m, nil
	}
	if m.entries[m.list.Cursor()].IsDir() {
		m.descend(name)
		return m, nil
	}
	path := filepath.Join(m.dir, name)
	return m, func() tea.Msg { return FileSelectedMsg{Path: path} }
}

// resolvePath handles a pasted/entered path: a directory descends into it; a
// file selects it; anything else is ignored.
func (m *FilePickerModel) resolvePath(p string) tea.Cmd {
	if p == "" {
		return nil
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil
	}
	if info.IsDir() {
		m.dir = p
		m.readDir()
		return nil
	}
	return func() tea.Msg { return FileSelectedMsg{Path: p} }
}

func (m *FilePickerModel) View() string {
	w := filePickerWidth
	if m.width > 0 && m.width < w {
		w = m.width
	}
	inner := w - 2

	rowFn := func(i int, selected bool) string {
		e := m.entries[i]
		label := e.Name()
		if e.IsDir() {
			label += "/"
		}
		style := lipgloss.NewStyle().Inline(true).Width(inner).MaxWidth(inner)
		if selected {
			style = style.Background(lipgloss.Color("63")).Foreground(lipgloss.Color("0"))
		}
		return style.Render(label)
	}
	lines := m.list.Render(filePickerMaxRows, rowFn)
	if len(m.entries) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Width(inner).Render("empty"))
	}

	hint := components.OverlayHint([][2]string{
		{"type", "filter"}, {"enter", "open"}, {"esc", "close"}, {"backspace", "up"},
	}, nil)
	content := strings.Join(lines, "\n")
	h := len(lines) + 2
	title := truncatePath(m.dir, inner)
	// The list rows are the whole content, so the scrollbar track starts at row 0.
	sb := m.list.Scrollbar(filePickerMaxRows, 0)
	return components.RenderBox(content, title, "", hint, "", lipgloss.RoundedBorder(), nil, w, h, sb)
}

func truncatePath(p string, max int) string {
	if lipgloss.Width(p) <= max {
		return p
	}
	r := []rune(p)
	keep := max - 1
	if keep < 0 {
		keep = 0
	}
	return "…" + string(r[len(r)-keep:])
}
