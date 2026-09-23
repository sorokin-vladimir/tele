package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// reactionFailedMsg reports a refused reaction for presentation only: the owner
// has already restored the previous reactions.
type reactionFailedMsg struct {
	chatID int64
	msgID  int
	err    error
}

// deleteMsgFailedMsg reports a refused delete for presentation only: the owner
// has already restored the message.
type deleteMsgFailedMsg struct {
	chatID int64
	msgID  int
	err    error
}

// editMsgFailedMsg reports a refused edit for presentation only: the owner has
// already restored the text, so nothing here touches state. chatID is carried
// to place the highlight, which belongs only to the chat still on screen.
type editMsgFailedMsg struct {
	chatID int64
	msgID  int
	err    error
}

// forwardDoneMsg reports the outcome of a forward for the status line. The
// target's preview bump is the owner's, so nothing else travels here.
type forwardDoneMsg struct {
	toTitle string
	// err is nil on success. It carries the domain kind, so the failure is
	// rendered by the same path as every other error rather than by flags that
	// have to be kept in step with it.
	err error
}

// handleSendMsg hands the message to the durable queue and returns. Nothing is
// inserted into the store and nothing is guessed at: the queue's own entry is
// what appears on screen, so an unfinished send survives this process and is
// visible to every attached client rather than only to the one that typed it
// (#193).
func (m RootModel) handleSendMsg(msg screens.SendMsgRequest) (RootModel, tea.Cmd) {
	if m.owner == nil {
		return m, nil
	}
	ctx, owner := m.ctx, m.owner
	req := core.SendRequest{
		Ref:          core.NewRef(),
		ChatID:       msg.ChatID,
		Text:         msg.Text,
		Entities:     msg.Entities,
		ReplyToMsgID: msg.ReplyToMsgID,
	}
	return m, func() tea.Msg {
		if err := owner.Send(ctx, req); err != nil {
			return errStatus("send", err)
		}
		return nil
	}
}

// handleSendMedia hands the staged files to the durable queue and returns.
// Nothing is inserted into the store, no upload is driven here, and no bubble is
// guessed at: the queue's own entries are what appear on screen, so an
// unfinished send survives this process and is visible to every attached client
// rather than only to the one that staged it (#195).
//
// The whole staged queue goes in one request. How it is cut into albums is
// Telegram's rule, and therefore the owner's business.
func (m RootModel) handleSendMedia(msg screens.SendMediaRequest) (RootModel, tea.Cmd) {
	if m.owner == nil || len(m.pendingAttachments) == 0 {
		return m, nil
	}
	files := make([]core.MediaFile, 0, len(m.pendingAttachments))
	for _, a := range m.pendingAttachments {
		files = append(files, core.MediaFile{Path: a.path, SendAs: a.sendAs})
	}
	req := core.MediaSendRequest{
		Ref:          core.NewRef(),
		ChatID:       msg.ChatID,
		Files:        files,
		Caption:      msg.Caption,
		Entities:     msg.Entities,
		ReplyToMsgID: msg.ReplyToMsgID,
	}
	m.clearPendingAttachments()
	ctx, owner := m.ctx, m.owner
	return m, func() tea.Msg {
		if err := owner.SendMedia(ctx, req); err != nil {
			return errStatus("send", err)
		}
		return nil
	}
}

// handleUploadProgress moves a queued send's progress bar. It is an event, not
// state: a frame for a chat that is not open, or one dropped on the way, costs
// nothing — the next frame corrects it.
func (m RootModel) handleUploadProgress(p core.Progress) (RootModel, tea.Cmd) {
	if m.chat == nil || p.ChatID != m.currentChatID {
		return m, nil
	}
	var frac float64
	if p.Total > 0 {
		frac = float64(p.Done) / float64(p.Total)
	}
	m.chat.SetUploadProgress(p.Ref, p.Part, p.Parts, frac)
	return m, nil
}

// showOutboxReason names why the selected send failed, in the status bar. The
// bubble carries only a glyph, so this is where the reason lives once the toast
// that announced it has gone (#193).
//
// What it offers depends on what failed. A refusal is about the content, so
// repeating it unchanged repeats the refusal, and pointing at retry would send a
// person round a loop; discard is the way out. Every other terminal kind is
// about circumstances that can change — rights restored, a chat reachable
// again — so retry stays the suggestion there. The retry key works either way:
// this stops recommending it, not offering it (#224).
//
// The two hints name different keys because the two actions live in different
// places. Retry is bound directly; discard is destructive and therefore stays
// behind the context menu (#193), so its hint names the key that opens the menu
// rather than a key that would do nothing.
func (m *RootModel) showOutboxReason() {
	if m.chat == nil {
		return
	}
	entry, ok := m.chat.SelectedOutboxEntry()
	if !ok || entry.State != domain.OutboxFailed {
		return
	}
	action, verb := keys.ActionConfirm, " to retry"
	if entry.ErrKind == telerr.Rejected {
		action, verb = keys.ActionOpenContextMenu, " to discard"
	}
	m.statusBar.SetStatus("not sent: " + components.OutboxReason(entry) +
		" — " + m.keyMap.KeyFor(keys.ContextChat, action) + verb)
}

// handleRetryOutbox puts a failed send back in the queue. The entry's new state
// reaches the pane through the projection like every other change, so nothing
// is updated here (#193).
func (m RootModel) handleRetryOutbox(msg components.RetryOutboxRequest) (RootModel, tea.Cmd) {
	m.contextMenu = nil
	if m.owner == nil {
		return m, nil
	}
	owner, ref := m.owner, msg.Ref
	return m, func() tea.Msg {
		if err := owner.RetryOutbox(ref); err != nil {
			return errStatus("retry", err)
		}
		return nil
	}
}

// handleDiscardOutbox drops a queued send. The text goes with it: that is what
// the action is for, and it is the only way an entry leaves the queue unsent.
func (m RootModel) handleDiscardOutbox(msg components.DiscardOutboxRequest) (RootModel, tea.Cmd) {
	m.contextMenu = nil
	if m.owner == nil {
		return m, nil
	}
	owner, ref := m.owner, msg.Ref
	return m, func() tea.Msg {
		if err := owner.DiscardOutbox(ref); err != nil {
			return errStatus("discard", err)
		}
		return nil
	}
}

func (m RootModel) handleEditSend(msg screens.EditSendRequest) (RootModel, tea.Cmd) {
	if m.owner == nil {
		return m, nil
	}
	ctx, owner, chatID := m.ctx, m.owner, msg.ChatID
	msgID, text, entities := msg.MsgID, msg.Text, msg.Entities
	return m, func() tea.Msg {
		if err := owner.EditMessage(ctx, chatID, msgID, text, entities); err != nil {
			return editMsgFailedMsg{chatID: chatID, msgID: msgID, err: err}
		}
		return nil
	}
}

func (m RootModel) handleEditMsgFailed(msg editMsgFailedMsg) (RootModel, tea.Cmd) {
	toast := func() tea.Msg { return errStatus("edit", msg.err) }
	if msg.chatID != m.currentChatID {
		return m, toast
	}
	return m.flashRollback(msg.msgID, toast)
}

// flashRollback scrolls the open chat to msgID, starts a red error highlight on
// it, and returns a command batching the failure toast with the highlight fade
// tick. Call only when the failing chat is the current chat.
func (m RootModel) flashRollback(msgID int, toast tea.Cmd) (RootModel, tea.Cmd) {
	m.chat.ScrollToMessage(msgID)
	m.chat.HighlightMessageError(msgID)
	m.msgHighlightSerial++
	return m, tea.Batch(toast, msgHighlightFadeCmd(m.msgHighlightSerial))
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
	return m.saveDraftCmd(m.currentChatID, text)
}

// saveDraftCmd returns a managed Cmd that saves (or clears, when text == "")
// a chat's draft through the owner, which stores it locally either way.
func (m RootModel) saveDraftCmd(chatID int64, text string) tea.Cmd {
	if m.owner == nil {
		return nil
	}
	appCtx, owner := m.ctx, m.owner
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(appCtx, 5*time.Second)
		defer cancel()
		// A failed draft sync is not worth interrupting for: the text is kept
		// locally and the next flush retries.
		_ = owner.SaveDraft(ctx, chatID, text)
		return nil
	}
}

func (m RootModel) handleSetTyping(msg screens.SetTypingRequest) (RootModel, tea.Cmd) {
	if m.owner == nil || msg.ChatID == 0 {
		return m, nil
	}
	appCtx, owner, chatID := m.ctx, m.owner, msg.ChatID
	action := msg.Action
	// Run as a managed tea.Cmd (not a detached goroutine) so the RPC is bound to
	// the app lifecycle context and cancelled on shutdown.
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(appCtx, 5*time.Second)
		defer cancel()
		// Typing notices are best-effort; a toast per failure would be noise.
		_ = owner.SetTyping(ctx, chatID, action)
		return nil
	}
}

func (m RootModel) handleReactConfirmed(msg components.ReactConfirmedMsg) (RootModel, tea.Cmd) {
	m.reactionPicker = nil
	if m.owner == nil {
		return m, nil
	}
	ctx, owner, chatID := m.ctx, m.owner, m.currentChatID
	msgID, emoji := m.reactionTargetID, msg.Emoji
	return m, func() tea.Msg {
		if err := owner.SendReaction(ctx, chatID, msgID, emoji); err != nil {
			return reactionFailedMsg{chatID: chatID, msgID: msgID, err: err}
		}
		return nil
	}
}

func (m RootModel) handleReactionFailed(msg reactionFailedMsg) (RootModel, tea.Cmd) {
	toast := func() tea.Msg { return errStatus("reaction", msg.err) }
	if msg.chatID != m.currentChatID {
		return m, toast
	}
	return m.flashRollback(msg.msgID, toast)
}

func (m RootModel) handleDeleteMsg(msg components.DeleteMsgRequest) (RootModel, tea.Cmd) {
	m.contextMenu = nil
	if m.owner == nil {
		return m, nil
	}
	ctx, owner, chatID := m.ctx, m.owner, m.currentChatID
	msgID, revoke := msg.MsgID, msg.Revoke
	return m, func() tea.Msg {
		if err := owner.DeleteMessages(ctx, chatID, []int{msgID}, revoke); err != nil {
			return deleteMsgFailedMsg{chatID: chatID, msgID: msgID, err: err}
		}
		return nil
	}
}

func (m RootModel) handleDeleteMsgFailed(msg deleteMsgFailedMsg) (RootModel, tea.Cmd) {
	toast := func() tea.Msg { return errStatus("delete", msg.err) }
	if msg.chatID != m.currentChatID {
		return m, toast
	}
	return m.flashRollback(msg.msgID, toast)
}

// buildOptimisticReactions is gone: what a reaction looks like before the
// server answers is state, so it moved to core as optimisticReactions (#198).

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
	serial := m.toasts.Add(components.ToastKindOf(msg.Sev), msg.Text)
	d := durationFor(msg.Sev)
	return m, tea.Tick(d, func(time.Time) tea.Msg { return ClearStatusErrMsg{Serial: serial} })
}

// handleDocumentOpenDone clears the status-bar download indicator for the
// completed external open and on failure surfaces the error (with the usual
// auto-clear timer). A refreshed file reference is recorded by the owner, not
// here (#196).
func (m RootModel) handleDocumentOpenDone(msg documentOpenDoneMsg) (RootModel, tea.Cmd) {
	m.statusBar.ClearDownload(msg.serial)
	if msg.errText != "" {
		serial := m.toasts.Add(components.ToastKindOf(msg.sev), msg.errText)
		d := durationFor(msg.sev)
		return m, tea.Tick(d, func(time.Time) tea.Msg { return ClearStatusErrMsg{Serial: serial} })
	}
	return m, nil
}

// handleFileDownloadDone clears the status-bar download indicator and surfaces
// the result (saved path or error) with the usual auto-clear timer. A refreshed
// file reference is recorded by the owner, not here (#196).
func (m RootModel) handleFileDownloadDone(msg fileDownloadDoneMsg) (RootModel, tea.Cmd) {
	m.statusBar.ClearDownload(msg.serial)
	// A cancelled download has nothing to say; the indicator is already cleared.
	if msg.text == "" {
		return m, nil
	}
	serial := m.toasts.Add(components.ToastKindOf(msg.sev), msg.text)
	d := durationFor(msg.sev)
	return m, tea.Tick(d, func(time.Time) tea.Msg { return ClearStatusErrMsg{Serial: serial} })
}

// retryChatLoadMsg re-triggers loading a chat's history after a load failure,
// emitted by a toast retry action (#87).
type retryChatLoadMsg struct{ chatID int64 }

func (m RootModel) handleChatLoadErr(msg chatLoadErrMsg) (RootModel, tea.Cmd) {
	// A cancelled load is not a failure and must not blank out the open chat
	// with an empty error banner.
	if msg.text == "" {
		return m, nil
	}
	if msg.chatID == m.currentChatID {
		m.chat.SetLoading(false)
		m.chat.SetLoadError(msg.text)
	}
	serial := m.toasts.Add(components.ToastError, msg.text,
		components.ToastAction{Label: "retry", Key: "r", Msg: retryChatLoadMsg{chatID: msg.chatID}})
	return m, tea.Tick(durationFor(components.SeverityError), func(time.Time) tea.Msg {
		return ClearStatusErrMsg{Serial: serial}
	})
}

// retryChatLoadCmd is gone: retrying a failed chat load is a resubscribe, which
// the model does directly. See the retryChatLoadMsg case in root.go.

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

// handleForwardToChat closes the picker and forwards the message from the open
// chat to the chosen target chat, surfacing the result via a status message.
func (m RootModel) handleForwardToChat(msg screens.ForwardToChatRequest) (RootModel, tea.Cmd) {
	m.searchModel = nil
	if m.owner == nil {
		return m, nil
	}
	ctx, owner, from := m.ctx, m.owner, m.currentChatID
	to, toTitle := msg.ToChatID, msg.Title
	ids, comment := []int{msg.MsgID}, msg.Comment
	m.debug("forward: client asked",
		zap.Int64("from_chat", from), zap.Int64("to_chat", to),
		zap.Ints("msg_ids", ids), zap.Bool("with_comment", comment != ""))
	return m, func() tea.Msg {
		err := owner.Forward(ctx, from, to, ids, comment)
		return forwardDoneMsg{toTitle: toTitle, err: err}
	}
}

// handleForwardDone turns a completed forward into a status message. The owner
// has already surfaced the target chat, so there is nothing to apply here.
func (m RootModel) handleForwardDone(msg forwardDoneMsg) (RootModel, tea.Cmd) {
	if msg.err != nil {
		text, sev, ok := errText("forward", msg.err)
		if !ok {
			return m, nil
		}
		return m, func() tea.Msg { return StatusErrMsg{Text: text, Sev: sev} }
	}
	m.statusBar.SetStatus("Forwarded to " + msg.toTitle)
	return m, nil
}

// activateReply sets reply state for msgID, switches to insert mode, and returns the FocusComposer cmd.
// Returns nil if msgID is zero.
func (m *RootModel) activateReply(msgID int) tea.Cmd {
	if msgID == 0 {
		return nil
	}
	preview := "▌ Reply to message"
	senderName := ""
	if m.st != nil {
		for _, storeMsg := range m.st.Messages(m.currentChatID) {
			if storeMsg.ID == msgID {
				preview = components.BuildReplyPreview(storeMsg)
				senderName = storeMsg.SenderName
				break
			}
		}
	}
	m.chat.SetReply(msgID, preview, senderName)
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
			m.chat.SetComposerSource(storeMsg.Text, storeMsg.Entities)
			m.vimState.Mode = keys.ModeInsert
			m.statusBar.SetMode(keys.ModeInsert)
			return m.chat.FocusComposer()
		}
	}
	return nil
}
