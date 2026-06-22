package ui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
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

type forwardDoneMsg struct {
	toTitle    string
	restricted bool
	failed     bool
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

// SetComposerValueForTest sets the open chat's composer text (tests only).
func (m RootModel) SetComposerValueForTest(s string) RootModel {
	m.chat.SetComposerValue(s)
	return m
}

// flushCurrentDraftCmd persists the open chat's composer text as a Telegram
// draft when it differs from the server-known value (#62). It updates the store
// (so a re-open shows the same text) and returns a Cmd performing the RPC, or
// nil when there is nothing to do. Edit mode is skipped: the composer then holds
// a message being edited, not a draft, and entering edit already discarded any
// prior draft.
func (m RootModel) flushCurrentDraftCmd() tea.Cmd {
	if m.st == nil || m.currentChatID == 0 || m.chat.EditMsgID() != 0 {
		return nil
	}
	chat, ok := m.st.GetChat(m.currentChatID)
	if !ok {
		return nil
	}
	text := m.chat.ComposerValue()
	if text == chat.Draft {
		return nil // unchanged — avoid a redundant messages.saveDraft round-trip
	}
	m.st.SetChatDraft(m.currentChatID, text)
	return m.saveDraftCmd(chat.Peer, text)
}

// saveDraftCmd returns a managed Cmd that saves (or clears, when text == "")
// the draft for a peer via the Telegram client. nil client → nil Cmd.
func (m RootModel) saveDraftCmd(peer store.Peer, text string) tea.Cmd {
	if m.tgClient == nil {
		return nil
	}
	appCtx := m.ctx
	client := m.tgClient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(appCtx, 5*time.Second)
		defer cancel()
		_ = client.SaveDraft(ctx, peer, text)
		return nil
	}
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

func durationFor(sev components.Severity) time.Duration {
	switch sev {
	case components.SeverityError:
		return 10 * time.Second
	case components.SeverityWarning:
		return 8 * time.Second
	default:
		return 5 * time.Second
	}
}

func (m RootModel) handleStatusErr(msg StatusErrMsg) (RootModel, tea.Cmd) {
	serial := m.statusBar.SetError(msg.Text, msg.Sev)
	d := durationFor(msg.Sev)
	return m, tea.Tick(d, func(time.Time) tea.Msg { return ClearStatusErrMsg{Serial: serial} })
}

// handleDocumentOpenDone clears the status-bar download indicator for the
// completed external open, persists any refreshed ref, and on failure surfaces
// the error (with the usual auto-clear timer).
func (m RootModel) handleDocumentOpenDone(msg documentOpenDoneMsg) (RootModel, tea.Cmd) {
	m.statusBar.ClearDownload(msg.serial)
	if msg.doc != nil && m.st != nil {
		m.st.UpdateMessageMedia(msg.chatID, msg.msgID, nil, msg.doc)
	}
	if msg.errText != "" {
		serial := m.statusBar.SetError(msg.errText, msg.sev)
		d := durationFor(msg.sev)
		return m, tea.Tick(d, func(time.Time) tea.Msg { return ClearStatusErrMsg{Serial: serial} })
	}
	return m, nil
}

// handleFileDownloadDone clears the status-bar download indicator, persists any
// refreshed ref, and surfaces the result (saved path or error) with the usual
// auto-clear timer.
func (m RootModel) handleFileDownloadDone(msg fileDownloadDoneMsg) (RootModel, tea.Cmd) {
	m.statusBar.ClearDownload(msg.serial)
	if msg.doc != nil && m.st != nil {
		m.st.UpdateMessageMedia(msg.chatID, msg.msgID, nil, msg.doc)
	}
	serial := m.statusBar.SetError(msg.text, msg.sev)
	d := durationFor(msg.sev)
	return m, tea.Tick(d, func(time.Time) tea.Msg { return ClearStatusErrMsg{Serial: serial} })
}

func (m RootModel) handleChatLoadErr(msg chatLoadErrMsg) (RootModel, tea.Cmd) {
	if msg.chatID == m.currentChatID {
		m.chat.SetLoading(false)
		m.chat.SetLoadError(msg.text)
	}
	serial := m.statusBar.SetError(msg.text, components.SeverityError)
	return m, tea.Tick(durationFor(components.SeverityError), func(time.Time) tea.Msg {
		return ClearStatusErrMsg{Serial: serial}
	})
}

// openReactionPicker opens the reaction picker for msgID, pre-selecting the
// already-chosen emoji (if any). No-op when there is no store or no message.
func (m RootModel) openReactionPicker(msgID int) RootModel {
	m.contextMenu = nil
	if m.st == nil || msgID == 0 {
		return m
	}
	var chosen string
	for _, sm := range m.st.Messages(m.currentChatID) {
		if sm.ID == msgID {
			for _, r := range sm.Reactions {
				if r.IsChosen {
					chosen = r.Emoji
					break
				}
			}
			break
		}
	}
	m.reactionTargetID = msgID
	m.reactionPicker = components.NewReactionPicker(chosen)
	return m
}

// openForwardPicker opens the fuzzy chat picker in forward mode for msgID.
// No-op (returns the model unchanged) when there is no store or no message.
func (m RootModel) openForwardPicker(msgID int) (RootModel, tea.Cmd) {
	if m.st == nil || msgID == 0 {
		return m, nil
	}
	m.contextMenu = nil
	m.searchModel = screens.NewForwardPicker(m.st.Chats(), msgID, m.width, m.height, m.keyMap)
	return m, nil
}

// handleForwardToChat closes the picker and forwards the message from the open
// chat to the chosen target peer, surfacing the result via a status message.
func (m RootModel) handleForwardToChat(msg screens.ForwardToChatRequest) (RootModel, tea.Cmd) {
	m.searchModel = nil
	if m.st == nil || m.tgClient == nil {
		return m, nil
	}
	chat, ok := m.st.GetChat(m.currentChatID)
	if !ok {
		return m, nil
	}
	var toTitle string
	if target, ok := m.st.GetChat(msg.ToPeer.ID); ok {
		toTitle = target.Title
	}
	ctx := m.ctx
	client := m.tgClient
	from := chat.Peer
	to := msg.ToPeer
	ids := []int{msg.MsgID}
	comment := msg.Comment
	return m, func() tea.Msg {
		if comment != "" {
			if _, err := client.SendMessage(ctx, to, comment, 0); err != nil {
				return forwardDoneMsg{toTitle: toTitle, failed: true}
			}
		}
		err := client.ForwardMessages(ctx, from, to, ids)
		switch {
		case err == nil:
			return forwardDoneMsg{toTitle: toTitle}
		case errors.Is(err, internaltg.ErrForwardRestricted):
			return forwardDoneMsg{toTitle: toTitle, restricted: true}
		default:
			return forwardDoneMsg{toTitle: toTitle, failed: true}
		}
	}
}

// handleForwardDone turns a completed forward into a status message.
func (m RootModel) handleForwardDone(msg forwardDoneMsg) (RootModel, tea.Cmd) {
	switch {
	case msg.restricted:
		return m, func() tea.Msg {
			return StatusErrMsg{Text: "forwarding restricted", Sev: components.SeverityWarning}
		}
	case msg.failed:
		return m, func() tea.Msg {
			return StatusErrMsg{Text: "forward failed", Sev: components.SeverityWarning}
		}
	default:
		m.statusBar.SetStatus("Forwarded to " + msg.toTitle)
		return m, nil
	}
}

// activateReply sets reply state for msgID, switches to insert mode, and returns the FocusComposer cmd.
// Returns nil if msgID is zero.
func (m *RootModel) activateReply(msgID int) tea.Cmd {
	if msgID == 0 {
		return nil
	}
	preview := "▌ Reply to message"
	if m.st != nil {
		for _, storeMsg := range m.st.Messages(m.currentChatID) {
			if storeMsg.ID == msgID {
				preview = components.BuildReplyPreview(storeMsg)
				break
			}
		}
	}
	m.chat.SetReply(msgID, preview)
	m.vimState.Mode = keys.ModeInsert
	m.statusBar.SetMode(keys.ModeInsert)
	return m.chat.FocusComposer()
}

// activateEdit sets edit state for msgID, pre-fills the composer with the
// original text, switches to insert mode, and returns the FocusComposer cmd.
// Returns nil if msgID is zero or the message is not found in the store.
func (m *RootModel) activateEdit(msgID int) tea.Cmd {
	if msgID == 0 {
		return nil
	}
	if m.st == nil {
		return nil
	}
	for _, storeMsg := range m.st.Messages(m.currentChatID) {
		if storeMsg.ID == msgID {
			preview := components.BuildEditPreview(storeMsg)
			m.chat.SetEdit(msgID, preview)
			m.chat.SetComposerValue(storeMsg.Text)
			m.vimState.Mode = keys.ModeInsert
			m.statusBar.SetMode(keys.ModeInsert)
			return m.chat.FocusComposer()
		}
	}
	return nil
}
