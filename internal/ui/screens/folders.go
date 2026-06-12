package screens

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
)

// FolderSelectedMsg is emitted when the user confirms a folder selection.
// Filter is nil when "All Chats" is selected.
type FolderSelectedMsg struct {
	Filter *store.FolderFilter
}

var (
	selectedFolderStyle = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("0"))
	normalFolderStyle   = lipgloss.NewStyle()
	activeFolderStyle   = lipgloss.NewStyle().Bold(true)
)

var allChatsFilter = store.FolderFilter{ID: 0, Title: "All Chats"}

var archiveFilter = store.FolderFilter{ID: store.ArchiveFolderID, Title: "Archive"}

type FoldersModel struct {
	folders      []store.FolderFilter // computed: [AllChats] + realFolders + (Archive?)
	realFolders  []store.FolderFilter
	showArchive  bool
	cursor       int
	activeIdx    int
	width        int
	height       int
	focused      bool
	unreadCounts map[int]int
}

func NewFoldersModel() *FoldersModel {
	m := &FoldersModel{unreadCounts: make(map[int]int)}
	m.rebuild()
	return m
}

func (m *FoldersModel) SetFolders(folders []store.FolderFilter) {
	m.realFolders = folders
	m.rebuild()
}

// SetArchivePresent shows or hides the Archive virtual folder entry. The
// Archive entry appears only while at least one archived chat exists, so an
// empty Archive is never drawn.
func (m *FoldersModel) SetArchivePresent(present bool) {
	if m.showArchive == present {
		return
	}
	m.showArchive = present
	m.rebuild()
}

// rebuild recomputes the folder list (All Chats, real folders, optional
// Archive) while preserving the cursor and active selection by filter ID.
func (m *FoldersModel) rebuild() {
	cursorID := m.idAt(m.cursor)
	activeID := m.idAt(m.activeIdx)

	folders := make([]store.FolderFilter, 0, len(m.realFolders)+2)
	folders = append(folders, allChatsFilter)
	folders = append(folders, m.realFolders...)
	if m.showArchive {
		folders = append(folders, archiveFilter)
	}
	m.folders = folders

	m.cursor = m.indexOfID(cursorID)
	m.activeIdx = m.indexOfID(activeID)
}

func (m *FoldersModel) idAt(idx int) int {
	if idx >= 0 && idx < len(m.folders) {
		return m.folders[idx].ID
	}
	return 0
}

func (m *FoldersModel) indexOfID(id int) int {
	for i, f := range m.folders {
		if f.ID == id {
			return i
		}
	}
	return 0 // fall back to All Chats
}

func (m *FoldersModel) SetFocused(focused bool)            { m.focused = focused }
func (m *FoldersModel) Focused() bool                      { return m.focused }
func (m *FoldersModel) SetSize(width, height int)          { m.width = width; m.height = height }
func (m *FoldersModel) SetUnreadCounts(counts map[int]int) { m.unreadCounts = counts }
func (m *FoldersModel) Cursor() int                        { return m.cursor }

// ScrollInfo reports the folders pane scroll position. The folders view does
// not currently clip/scroll, so Offset is always 0; the indicator is shown only
// if the list overflows the pane height (rare).
func (m *FoldersModel) ScrollInfo() components.ScrollInfo {
	return components.ScrollInfo{Total: len(m.folders), Visible: m.height, Offset: 0}
}
func (m *FoldersModel) Folders() []store.FolderFilter      { return m.folders }
func (m *FoldersModel) Context() keys.Context              { return keys.ContextFolders }

// SelectedFilter returns the currently active filter. Nil means All Chats.
func (m *FoldersModel) SelectedFilter() *store.FolderFilter {
	if m.activeIdx == 0 {
		return nil
	}
	f := m.folders[m.activeIdx]
	return &f
}

func (m *FoldersModel) HasFolders() bool {
	return len(m.folders) > 1
}

func (m *FoldersModel) Init() tea.Cmd { return nil }

func (m FoldersModel) Update(msg tea.Msg) (layout.Pane, tea.Cmd) {
	if !m.focused {
		return &m, nil
	}
	action, ok := msg.(keys.ActionMsg)
	if !ok {
		return &m, nil
	}
	switch action.Action {
	case keys.ActionDown:
		if m.cursor < len(m.folders)-1 {
			m.cursor++
		}
	case keys.ActionUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case keys.ActionConfirm:
		m.activeIdx = m.cursor
		var f *store.FolderFilter
		if m.activeIdx > 0 {
			ff := m.folders[m.activeIdx]
			f = &ff
		}
		return &m, func() tea.Msg { return FolderSelectedMsg{Filter: f} }
	}
	return &m, nil
}

func (m FoldersModel) View() string {
	var lines []string
	for i, f := range m.folders {
		label := m.formatEntry(f, i == m.activeIdx)
		style := normalFolderStyle
		if i == m.cursor && m.focused {
			style = selectedFolderStyle
		} else if i == m.activeIdx {
			style = activeFolderStyle
		}
		lines = append(lines, style.Render(label))
	}
	return joinLines(lines)
}

func (m FoldersModel) formatEntry(f store.FolderFilter, active bool) string {
	badge := ""
	if f.ID != 0 {
		if n, ok := m.unreadCounts[f.ID]; ok && n > 0 {
			if n > 99 {
				badge = "[99+]"
			} else {
				badge = fmt.Sprintf("[%d]", n)
			}
		}
	}
	prefix := "  "
	if active {
		prefix = "▶ "
	}
	nameWidth := m.width - lipgloss.Width(prefix) - lipgloss.Width(badge)
	if badge != "" {
		nameWidth-- // reserve 1 col for separator space
	}
	if nameWidth < 1 {
		nameWidth = 1
	}
	name := runewidth.Truncate(f.Title, nameWidth, "…")
	line := prefix + padRight(name, nameWidth)
	if badge != "" {
		line += " " + badge
	}
	return line
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	for i := w; i < width; i++ {
		s += " "
	}
	return s
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
