package ui_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestDetailsKey_TogglesExpandedAndResetsOnChatSwitch(t *testing.T) {
	m, st := newRootOnChat(t)
	st.SetChat(domain.Chat{ID: 2, Title: "Two", Peer: domain.Peer{ID: 2, Type: domain.PeerUser}})
	st.AppendMessage(domain.Message{
		ID:     7,
		ChatID: 1,
		Date:   time.Now(),
		Rich: &domain.RichMessage{
			Blocks: []domain.RichBlock{{
				Kind:  domain.RichDetails,
				Title: domain.RichText{Kind: domain.RichTextPlain, Text: "More info"},
				Blocks: []domain.RichBlock{{
					Kind: domain.RichParagraph,
					Text: domain.RichText{Kind: domain.RichTextPlain, Text: "hidden body payload"},
				}},
			}},
		},
	})
	nm, _ := applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)

	collapsed := xansi.Strip(m.View().Content)
	assert.Contains(t, collapsed, "▸ More info")
	assert.NotContains(t, collapsed, "hidden body payload")

	// Pressing z expands details across the chat.
	nm, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m = nm.(ui.RootModel)
	expanded := xansi.Strip(m.View().Content)
	assert.Contains(t, expanded, "▾ More info")
	assert.Contains(t, expanded, "hidden body payload")
	assert.Contains(t, expanded, "Details expanded")

	// Pressing z again collapses them.
	nm, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m = nm.(ui.RootModel)
	recollapsed := xansi.Strip(m.View().Content)
	assert.Contains(t, recollapsed, "▸ More info")
	assert.NotContains(t, recollapsed, "hidden body payload")
	assert.Contains(t, recollapsed, "Details collapsed")

	// Expand again, then switch to another chat: details start collapsed again.
	nm, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m = nm.(ui.RootModel)
	assert.Contains(t, xansi.Strip(m.View().Content), "hidden body payload")

	m = openChat(t, m, 2, "Two")
	m = openChat(t, m, 1, "Alice")
	nm, _ = applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)
	afterSwitch := xansi.Strip(m.View().Content)
	assert.Contains(t, afterSwitch, "▸ More info")
	assert.NotContains(t, afterSwitch, "hidden body payload")
}
