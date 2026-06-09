package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

type sentMsgConfirmedMsg struct {
	chatID     int64
	sentinelID int
	realID     int
	failed     bool
}

type reactionFailedMsg struct {
	chatID    int64
	msgID     int
	reactions []store.Reaction
}

type deleteMsgFailedMsg struct {
	chatID   int64
	messages []store.Message
}

type editMsgFailedMsg struct {
	chatID   int64
	messages []store.Message
}

func (m RootModel) handleSendMsg(msg screens.SendMsgRequest) (RootModel, tea.Cmd) {
	if m.tgClient == nil {
		return m, nil
	}
	m.nextSentinel--
	sentinelID := m.nextSentinel
	sentinel := store.Message{
		ID:           sentinelID,
		ChatID:       m.currentChatID,
		Text:         msg.Text,
		Date:         time.Now(),
		IsOut:        true,
		ReplyToMsgID: msg.ReplyToMsgID,
	}
	if m.st != nil {
		m.st.AppendMessage(sentinel)
		m.chat.SetMessages(m.st.Messages(m.currentChatID))
	}
	ctx := m.ctx
	client := m.tgClient
	peer := msg.Peer
	text := msg.Text
	replyToMsgID := msg.ReplyToMsgID
	chatID := m.currentChatID
	return m, func() tea.Msg {
		realID, err := client.SendMessage(ctx, peer, text, replyToMsgID)
		if err != nil {
			return sentMsgConfirmedMsg{chatID: chatID, sentinelID: sentinelID, realID: 0, failed: true}
		}
		return sentMsgConfirmedMsg{chatID: chatID, sentinelID: sentinelID, realID: realID}
	}
}

func (m RootModel) handleSentMsgConfirmed(msg sentMsgConfirmedMsg) (RootModel, tea.Cmd) {
	if m.st == nil {
		return m, nil
	}
	if msg.realID != 0 {
		m.st.UpdateMessageID(msg.chatID, msg.sentinelID, msg.realID)
	} else {
		m.st.RemoveMessage(msg.chatID, msg.sentinelID)
	}
	if msg.chatID == m.currentChatID {
		m.chat.SetMessages(m.st.Messages(msg.chatID))
	}
	if msg.failed {
		return m, func() tea.Msg { return StatusErrMsg{Text: "send failed", Sev: components.SeverityWarning} }
	}
	return m, nil
}

func (m RootModel) handleEditSend(msg screens.EditSendRequest) (RootModel, tea.Cmd) {
	if m.st == nil || m.tgClient == nil {
		return m, nil
	}
	chatID := m.currentChatID
	origMessages := m.st.Messages(chatID)
	m.st.UpdateMessageText(chatID, msg.MsgID, msg.Text, time.Now())
	m.chat.SetMessages(m.st.Messages(chatID))
	ctx := m.ctx
	client := m.tgClient
	peer := msg.Peer
	msgID := msg.MsgID
	text := msg.Text
	return m, func() tea.Msg {
		if err := client.EditMessage(ctx, peer, msgID, text); err != nil {
			return editMsgFailedMsg{chatID: chatID, messages: origMessages}
		}
		return nil
	}
}

func (m RootModel) handleEditMsgFailed(msg editMsgFailedMsg) (RootModel, tea.Cmd) {
	if m.st != nil {
		m.st.SetMessages(msg.chatID, msg.messages)
		if msg.chatID == m.currentChatID {
			m.chat.SetMessagesKeepScroll(m.st.Messages(msg.chatID))
		}
	}
	return m, func() tea.Msg { return StatusErrMsg{Text: "edit failed", Sev: components.SeverityWarning} }
}

func (m RootModel) handleSetTyping(msg screens.SetTypingRequest) (RootModel, tea.Cmd) {
	if m.tgClient == nil {
		return m, nil
	}
	appCtx := m.ctx
	client := m.tgClient
	peer := msg.Peer
	action := msg.Action
	// Run as a managed tea.Cmd (not a detached goroutine) so the RPC is bound to
	// the app lifecycle context and cancelled on shutdown.
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(appCtx, 5*time.Second)
		defer cancel()
		_ = client.SetTyping(ctx, peer, action)
		return nil
	}
}

func (m RootModel) handleReactConfirmed(msg components.ReactConfirmedMsg) (RootModel, tea.Cmd) {
	m.reactionPicker = nil
	if m.st == nil || m.tgClient == nil {
		return m, nil
	}
	chatID := m.currentChatID
	msgID := m.reactionTargetID
	emoji := msg.Emoji
	currentReactions := m.st.Messages(chatID)
	var msgReactions []store.Reaction
	for _, sm := range currentReactions {
		if sm.ID == msgID {
			msgReactions = sm.Reactions
			break
		}
	}
	alreadyChosen := false
	for _, r := range msgReactions {
		if r.Emoji == emoji && r.IsChosen {
			alreadyChosen = true
			break
		}
	}
	sendEmoji := emoji
	if alreadyChosen {
		sendEmoji = ""
	}
	origReactions := make([]store.Reaction, len(msgReactions))
	copy(origReactions, msgReactions)
	newReactions := buildOptimisticReactions(msgReactions, emoji)
	m.st.UpdateMessageReactions(chatID, msgID, newReactions)
	m.chat.SetMessagesKeepScroll(m.st.Messages(chatID))
	chat, ok := m.st.GetChat(chatID)
	if !ok {
		return m, nil
	}
	ctx := m.ctx
	client := m.tgClient
	peer := chat.Peer
	return m, func() tea.Msg {
		if err := client.SendReaction(ctx, peer, msgID, sendEmoji); err != nil {
			return reactionFailedMsg{chatID: chatID, msgID: msgID, reactions: origReactions}
		}
		return nil
	}
}

func (m RootModel) handleReactionFailed(msg reactionFailedMsg) (RootModel, tea.Cmd) {
	if m.st != nil {
		m.st.UpdateMessageReactions(msg.chatID, msg.msgID, msg.reactions)
		if msg.chatID == m.currentChatID {
			m.chat.SetMessagesKeepScroll(m.st.Messages(msg.chatID))
		}
	}
	return m, func() tea.Msg { return StatusErrMsg{Text: "reaction failed", Sev: components.SeverityWarning} }
}

func (m RootModel) handleDeleteMsg(msg components.DeleteMsgRequest) (RootModel, tea.Cmd) {
	m.contextMenu = nil
	if m.st == nil {
		return m, nil
	}
	chatID := m.currentChatID
	origMessages := m.st.Messages(chatID)
	m.st.RemoveMessage(chatID, msg.MsgID)
	m.chat.RemoveMessage(msg.MsgID)
	if m.tgClient == nil {
		return m, nil
	}
	chat, ok := m.st.GetChat(chatID)
	if !ok {
		return m, nil
	}
	ctx := m.ctx
	client := m.tgClient
	peer := chat.Peer
	msgID := msg.MsgID
	revoke := msg.Revoke
	return m, func() tea.Msg {
		if err := client.DeleteMessages(ctx, peer, []int{msgID}, revoke); err != nil {
			return deleteMsgFailedMsg{chatID: chatID, messages: origMessages}
		}
		return nil
	}
}

func (m RootModel) handleDeleteMsgFailed(msg deleteMsgFailedMsg) (RootModel, tea.Cmd) {
	if m.st != nil {
		m.st.SetMessages(msg.chatID, msg.messages)
		if msg.chatID == m.currentChatID {
			m.chat.SetMessagesKeepScroll(m.st.Messages(msg.chatID))
		}
	}
	return m, func() tea.Msg { return StatusErrMsg{Text: "delete failed", Sev: components.SeverityWarning} }
}

func buildOptimisticReactions(current []store.Reaction, emoji string) []store.Reaction {
	alreadyChosen := false
	for _, r := range current {
		if r.Emoji == emoji && r.IsChosen {
			alreadyChosen = true
			break
		}
	}
	out := make([]store.Reaction, 0, len(current)+1)
	emojiFound := false
	for _, r := range current {
		nr := r
		if r.Emoji == emoji {
			emojiFound = true
			if alreadyChosen {
				nr.IsChosen = false
				nr.Count--
				if nr.Count <= 0 {
					continue
				}
			} else {
				nr.IsChosen = true
				nr.Count++
			}
		} else if r.IsChosen {
			nr.IsChosen = false
			nr.Count--
			if nr.Count <= 0 {
				continue
			}
		}
		out = append(out, nr)
	}
	if !alreadyChosen && !emojiFound && emoji != "" {
		out = append(out, store.Reaction{Emoji: emoji, Count: 1, IsChosen: true})
	}
	return out
}
