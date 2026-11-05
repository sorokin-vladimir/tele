package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/textwidth"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// MentionSelectedMsg is emitted when the user picks a member from the popup.
type MentionSelectedMsg struct{ Member domain.ChatMember }

// CloseMentionPopupMsg is emitted when the popup is dismissed without a pick.
type CloseMentionPopupMsg struct{}

const (
	mentionPopupMaxRows  = 6
	mentionPopupMinWidth = 16
)

// MentionPopup is a keyboard-driven autocomplete overlay listing chat members
// for the composer's @mention trigger.
type MentionPopup struct {
	all      []domain.ChatMember
	filtered []domain.ChatMember
	cursor   int
	offset   int // index of the first rendered row (scroll window top)
	loading  bool
	width    int
}

func NewMentionPopup() *MentionPopup { return &MentionPopup{} }

func (p *MentionPopup) SetWidth(w int)    { p.width = w }
func (p *MentionPopup) SetLoading(v bool) { p.loading = v }

func (p *MentionPopup) SetMembers(all []domain.ChatMember) {
	p.all = all
	p.filtered = all
	p.cursor = 0
	p.offset = 0
}

// clampScroll keeps the cursor within the visible window [offset, offset+maxRows).
func (p *MentionPopup) clampScroll() {
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+mentionPopupMaxRows {
		p.offset = p.cursor - mentionPopupMaxRows + 1
	}
}

// Filter narrows the candidates to those whose username or display name contains
// query (case-insensitive) and resets the cursor.
func (p *MentionPopup) Filter(query string) {
	q := strings.ToLower(query)
	if q == "" {
		p.filtered = p.all
		p.cursor = 0
		p.offset = 0
		return
	}
	out := make([]domain.ChatMember, 0, len(p.all))
	for _, m := range p.all {
		if strings.Contains(strings.ToLower(m.Username), q) ||
			strings.Contains(strings.ToLower(m.DisplayName), q) {
			out = append(out, m)
		}
	}
	p.filtered = out
	p.cursor = 0
	p.offset = 0
}

func (p *MentionPopup) Filtered() []domain.ChatMember { return p.filtered }
func (p *MentionPopup) Empty() bool                   { return len(p.filtered) == 0 }

func (p *MentionPopup) Update(msg tea.Msg) (*MentionPopup, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	switch keys.NormalizeKey(kp.String()) {
	case "esc":
		return p, func() tea.Msg { return CloseMentionPopupMsg{} }
	case "k", "up", "ctrl+k":
		if p.cursor > 0 {
			p.cursor--
			p.clampScroll()
		}
	case "j", "down", "ctrl+j":
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
			p.clampScroll()
		}
	case "enter", "tab":
		if p.cursor >= 0 && p.cursor < len(p.filtered) {
			m := p.filtered[p.cursor]
			return p, func() tea.Msg { return MentionSelectedMsg{Member: m} }
		}
	}
	return p, nil
}

// mentionRow holds one candidate's display parts for width measurement and
// styling. name and handle are plain (unstyled) so widths are accurate.
type mentionRow struct {
	name   string
	handle string // "@username" or ""
}

func (p *MentionPopup) View() string {
	// Status rows (loading / no matches) render as a single non-selectable line.
	if p.loading || len(p.filtered) == 0 {
		text := "loading members…"
		if !p.loading {
			text = "no matches"
		}
		return p.box([]string{theme.S().MentionStatus.Render(" " + text + " ")}, textwidth.Width(text)+2)
	}

	// Render only the visible window [offset, offset+maxRows) so the cursor stays
	// on screen for long lists.
	start := p.offset
	end := start + mentionPopupMaxRows
	if end > len(p.filtered) {
		end = len(p.filtered)
	}
	rows := make([]mentionRow, 0, end-start)
	innerW := mentionPopupMinWidth
	for i := start; i < end; i++ {
		m := p.filtered[i]
		r := mentionRow{name: m.DisplayName}
		if m.Username != "" {
			r.handle = "@" + m.Username
		}
		rows = append(rows, r)
		// " name  @handle " with one-space gutters and a two-space separator.
		w := 1 + textwidth.Width(r.name) + 1
		if r.handle != "" {
			w += 1 + textwidth.Width(r.handle)
		}
		if w > innerW {
			innerW = w
		}
	}

	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = p.renderRow(r, start+i == p.cursor, innerW)
	}
	return p.box(lines, innerW)
}

// renderRow lays out one candidate padded to innerW so the row background spans
// the full bubble width. The @handle is dimmed relative to the name.
func (p *MentionPopup) renderRow(r mentionRow, selected bool, innerW int) string {
	base, dim := theme.S().MentionRow, theme.S().MentionDim
	if selected {
		base, dim = theme.S().MentionSel, theme.S().MentionSelDim
	}
	// Compose the visible text, padding to innerW with the row background.
	used := 1 + textwidth.Width(r.name) // leading space + name
	handle := ""
	if r.handle != "" {
		handle = dim.Render(r.handle)
		used += 1 + textwidth.Width(r.handle) // separator space + handle
	}
	pad := innerW - used - 1 // trailing space
	if pad < 0 {
		pad = 0
	}
	var b strings.Builder
	b.WriteString(base.Render(" " + r.name))
	if handle != "" {
		b.WriteString(base.Render(" "))
		b.WriteString(handle)
	}
	// canvas:ok rendered through base, which paints the popup surface.
	b.WriteString(base.Render(strings.Repeat(" ", pad+1)))
	return b.String()
}

// box wraps pre-padded lines in a rounded border bubble innerW wide.
func (p *MentionPopup) box(lines []string, innerW int) string {
	content := strings.Join(lines, "\n")
	return RenderBox(content, "", "", "", "", lipgloss.RoundedBorder(), theme.T().SurfaceSelected, innerW+2, len(lines)+2)
}
