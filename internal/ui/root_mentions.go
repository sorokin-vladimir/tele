package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// participantsLoadedMsg carries the fetched mention candidates for a chat.
type participantsLoadedMsg struct {
	chatID  int64
	members []domain.ChatMember
}

// syncMentionPopup opens, refreshes, or closes the mention popup based on the
// composer's active @-query. Returns a Cmd to fetch participants when the
// current chat's members are not cached yet.
func (m *RootModel) syncMentionPopup() tea.Cmd {
	q, active := m.chat.ComposerMentionQuery()
	// Mentions target group/channel participants; a 1:1 chat has none, so never
	// open the popup there (and never fire a getFullChat on a user peer). A chat
	// whose header has not landed yet counts as no group: asking too late is
	// harmless, asking a person for members is not.
	if !active || !m.chat.IsGroup() {
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
	return m.fetchParticipantsCmd(chatID)
}

func (m RootModel) fetchParticipantsCmd(chatID int64) tea.Cmd {
	if m.owner == nil {
		return nil
	}
	ctx, owner := m.ctx, m.owner
	return func() tea.Msg {
		members, err := owner.GetParticipants(ctx, chatID)
		if err != nil {
			return participantsLoadedMsg{chatID: chatID, members: nil}
		}
		return participantsLoadedMsg{chatID: chatID, members: members}
	}
}

func (m RootModel) handleParticipantsLoaded(msg participantsLoadedMsg) (RootModel, tea.Cmd) {
	if m.mentionMembers == nil {
		m.mentionMembers = map[int64][]domain.ChatMember{}
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
