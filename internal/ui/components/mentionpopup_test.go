package components_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func mentionMembers() []store.ChatMember {
	return []store.ChatMember{
		{UserID: 1, Username: "alice", DisplayName: "Alice A"},
		{UserID: 2, Username: "bob", DisplayName: "Bob B"},
		{UserID: 3, Username: "", DisplayName: "Alan Nowhere"},
	}
}

func TestMentionPopupFilterByUsernameAndName(t *testing.T) {
	p := components.NewMentionPopup()
	p.SetMembers(mentionMembers())
	p.Filter("al")
	got := p.Filtered()
	if len(got) != 2 { // alice (username), Alan (name)
		t.Fatalf("want 2 candidates, got %d", len(got))
	}
}

func TestMentionPopupSelectEmitsMember(t *testing.T) {
	p := components.NewMentionPopup()
	p.SetMembers(mentionMembers())
	p.Filter("bob")
	_, cmd := p.Update(pressEnter())
	if cmd == nil {
		t.Fatal("Enter must emit a command")
	}
	sel, ok := cmd().(components.MentionSelectedMsg)
	if !ok || sel.Member.UserID != 2 {
		t.Fatalf("want MentionSelectedMsg for bob, got %#v", cmd())
	}
}

func TestMentionPopupDownThenSelect(t *testing.T) {
	p := components.NewMentionPopup()
	p.SetMembers(mentionMembers())
	p.Filter("") // all three
	p, _ = p.Update(pressDown())
	_, cmd := p.Update(pressEnter())
	sel, ok := cmd().(components.MentionSelectedMsg)
	if !ok || sel.Member.UserID != 2 {
		t.Fatalf("want second member (bob) after down, got %#v", cmd())
	}
}

func TestMentionPopupCtrlJMovesDown(t *testing.T) {
	p := components.NewMentionPopup()
	p.SetMembers(mentionMembers())
	p.Filter("") // all three
	p, _ = p.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	_, cmd := p.Update(pressEnter())
	sel, ok := cmd().(components.MentionSelectedMsg)
	if !ok || sel.Member.UserID != 2 {
		t.Fatalf("want second member (bob) after ctrl+j, got %#v", cmd())
	}
}

func TestMentionPopupCtrlKMovesUp(t *testing.T) {
	p := components.NewMentionPopup()
	p.SetMembers(mentionMembers())
	p.Filter("")
	p, _ = p.Update(pressDown()) // cursor -> 1
	p, _ = p.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	_, cmd := p.Update(pressEnter())
	sel, ok := cmd().(components.MentionSelectedMsg)
	if !ok || sel.Member.UserID != 1 {
		t.Fatalf("want first member (alice) after ctrl+k, got %#v", cmd())
	}
}

func TestMentionPopupScrollsToKeepCursorVisible(t *testing.T) {
	var members []store.ChatMember
	for i := 1; i <= 8; i++ {
		members = append(members, store.ChatMember{
			UserID:      int64(i),
			DisplayName: "User" + string(rune('0'+i)),
		})
	}
	p := components.NewMentionPopup()
	p.SetMembers(members)
	p.Filter("")

	// Move to the last item (index 7); the 6-row window must scroll.
	for i := 0; i < 7; i++ {
		p, _ = p.Update(pressDown())
	}
	view := stripANSI(p.View())
	if !strings.Contains(view, "User8") {
		t.Fatalf("cursor item User8 must be visible after scrolling:\n%s", view)
	}
	if strings.Contains(view, "User1") {
		t.Fatalf("top item User1 must have scrolled out of the window:\n%s", view)
	}

	// Enter selects the item the cursor is on, even though it started off-screen.
	_, cmd := p.Update(pressEnter())
	sel, ok := cmd().(components.MentionSelectedMsg)
	if !ok || sel.Member.UserID != 8 {
		t.Fatalf("want User8 selected, got %#v", cmd())
	}
}

func TestMentionPopupEscCloses(t *testing.T) {
	p := components.NewMentionPopup()
	p.SetMembers(mentionMembers())
	_, cmd := p.Update(pressEsc())
	if cmd == nil {
		t.Fatal("Esc must emit a command")
	}
	if _, ok := cmd().(components.CloseMentionPopupMsg); !ok {
		t.Fatalf("Esc must emit CloseMentionPopupMsg, got %#v", cmd())
	}
}

func TestMentionPopupBoxShape(t *testing.T) {
	p := components.NewMentionPopup()
	p.SetMembers([]store.ChatMember{
		{UserID: 1, Username: "sorokin_vl", DisplayName: "Vladimir"},
		{UserID: 2, Username: "bloom_80", DisplayName: "Igor Bloom"},
		{UserID: 3, DisplayName: "Сергей"},
		{UserID: 4, Username: "turan95", DisplayName: "Turan"},
	})
	p.Filter("")
	lines := strings.Split(p.View(), "\n")

	if len(lines) != 6 { // 4 rows + 2 border rows
		t.Fatalf("want 6 lines, got %d", len(lines))
	}
	first := stripANSI(lines[0])
	last := stripANSI(lines[len(lines)-1])
	if !strings.HasPrefix(first, "╭") || !strings.HasSuffix(first, "╮") {
		t.Fatalf("top border malformed: %q", first)
	}
	if !strings.HasPrefix(last, "╰") || !strings.HasSuffix(last, "╯") {
		t.Fatalf("bottom border malformed: %q", last)
	}
	// Every line must share the same display width (a filled, aligned bubble).
	w0 := lipgloss.Width(lines[0])
	for i, l := range lines {
		if lipgloss.Width(l) != w0 {
			t.Fatalf("line %d width %d != %d (%q)", i, lipgloss.Width(l), w0, stripANSI(l))
		}
	}
}

func TestMentionPopupStatusBoxShape(t *testing.T) {
	p := components.NewMentionPopup()
	p.SetLoading(true)
	lines := strings.Split(p.View(), "\n")
	if len(lines) != 3 { // 1 status row + 2 border rows
		t.Fatalf("want 3 lines for loading box, got %d", len(lines))
	}
	w0 := lipgloss.Width(lines[0])
	for i, l := range lines {
		if lipgloss.Width(l) != w0 {
			t.Fatalf("status line %d width mismatch: %q", i, stripANSI(l))
		}
	}
}
