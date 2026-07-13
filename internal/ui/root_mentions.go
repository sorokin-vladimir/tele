package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// participantsLoadedMsg carries the fetched mention candidates for a chat.
type participantsLoadedMsg struct {
	chatID  int64
	members []store.ChatMember
}

// syncMentionPopup opens, refreshes, or closes the mention popup based on the
// composer's active @-query. Returns a Cmd to fetch participants when the
// current chat's members are not cached yet.
func (m *RootModel) syncMentionPopup() tea.Cmd {
	q, active := m.chat.ComposerMentionQuery()
	// Mentions target group/channel participants; a 1:1 chat has none, so never
	// open the popup there (and never fire a getFullChat on a user peer).
	if !active || m.chat.CurrentPeer().IsUser() {
		m.mentionPopup = nil
		return nil
	}
	if m.mentionPopup == nil {
		m.mentionPopup = components.NewMentionPopup()
	}
	chatID := m.currentChatID
	if members, ok := m.mentionMembers[chatID]; ok {
		m.mentionPopup.SetLoading(false)
		m.mentionPopup.SetMembers(members)
		m.mentionPopup.Filter(q)
		return nil
	}
	m.mentionPopup.SetLoading(true)
	return m.fetchParticipantsCmd(chatID, m.chat.CurrentPeer())
}

func (m RootModel) fetchParticipantsCmd(chatID int64, peer store.Peer) tea.Cmd {
	ctx := m.ctx
	client := m.tgClient
	return func() tea.Msg {
		members, err := client.GetParticipants(ctx, peer)
		if err != nil {
			return participantsLoadedMsg{chatID: chatID, members: nil}
		}
		return participantsLoadedMsg{chatID: chatID, members: members}
	}
}

func (m RootModel) handleParticipantsLoaded(msg participantsLoadedMsg) (RootModel, tea.Cmd) {
	if m.mentionMembers == nil {
		m.mentionMembers = map[int64][]store.ChatMember{}
	}
	m.mentionMembers[msg.chatID] = msg.members
	if m.mentionPopup != nil && msg.chatID == m.currentChatID {
		if q, active := m.chat.ComposerMentionQuery(); active {
			m.mentionPopup.SetLoading(false)
			m.mentionPopup.SetMembers(msg.members)
			m.mentionPopup.Filter(q)
		}
	}
	return m, nil
}

func (m RootModel) handleMentionSelected(msg components.MentionSelectedMsg) (RootModel, tea.Cmd) {
	m.chat.ApplyComposerMention(msg.Member)
	m.mentionPopup = nil
	return m, nil
}

func (m RootModel) handleCloseMentionPopup() (RootModel, tea.Cmd) {
	m.mentionPopup = nil
	return m, nil
}
