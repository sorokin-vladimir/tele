package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

type uploadProgressMsg struct {
	sentinelID int
	sent       int64
	total      int64
}

type sentMediaConfirmedMsg struct {
	chatID     int64
	sentinelID int
	realID     int
	failed     bool
}

// sentMediaRefreshedMsg carries the server's media refs for a just-sent message,
// fetched via RefreshMessage because the echoed update is suppressed (the photo
// would otherwise not appear until a manual refresh).
type sentMediaRefreshedMsg struct {
	chatID    int64
	msgID     int
	refreshed store.Message
	ok        bool
}

// mediaSendJob is the type-agnostic unit handleSendMedia consumes. Only kind and
// buildMedia are media-specific; #107 (video) and #129 (document) reuse the
// engine by supplying a different kind + builder.
type mediaSendJob struct {
	peer         store.Peer
	path         string
	name         string
	size         int64
	kind         store.MediaKind
	caption      string
	replyToMsgID int
	buildMedia   func(tg.InputFileClass) tg.InputMediaClass
	// buildMediaCtx, when set, takes precedence over buildMedia. It runs inside the
	// upload goroutine with client + ctx so video (#107) can probe metadata,
	// generate and upload a thumbnail, then build the InputMedia. nil for
	// photo/file, which need neither the client nor ctx.
	buildMediaCtx func(ctx context.Context, client internaltg.Client, main tg.InputFileClass) (tg.InputMediaClass, error)
}

// mediaBuilderFor returns the InputMedia builder for a staged attachment's "send
// as" kind. Photo and File (document) are implemented; video (#107), voice
// (#108) and round (#109) add their cases here. ok=false means not yet supported.
func mediaBuilderFor(att *pendingAttachment) (func(tg.InputFileClass) tg.InputMediaClass, bool) {
	switch att.sendAs {
	case store.MediaPhoto:
		return internaltg.BuildInputMediaUploadedPhoto, true
	case store.MediaFile:
		name, mime := att.name, att.mime
		return func(f tg.InputFileClass) tg.InputMediaClass {
			return internaltg.BuildInputMediaUploadedDocument(f, name, mime)
		}, true
	default:
		return nil, false
	}
}

func (m RootModel) handleSendMedia(job mediaSendJob) (RootModel, tea.Cmd) {
	if m.tgClient == nil || m.st == nil || (job.buildMedia == nil && job.buildMediaCtx == nil) {
		return m, nil
	}
	m.nextSentinel--
	sentinelID := m.nextSentinel
	chatID := m.currentChatID
	sentinel := store.Message{
		ID:           sentinelID,
		ChatID:       chatID,
		Text:         job.caption,
		Date:         time.Now(),
		IsOut:        true,
		ReplyToMsgID: job.replyToMsgID,
		LocalMedia: &store.LocalMedia{
			Path:        job.path,
			Kind:        job.kind,
			FileName:    job.name,
			Size:        job.size,
			UploadState: store.UploadUploading,
		},
	}
	m.st.AppendMessage(sentinel)
	m.chat.SetMessages(m.st.Messages(chatID))

	uploadCtx, cancel := context.WithCancel(m.ctx)
	m.uploadCancels[sentinelID] = cancel

	client := m.tgClient
	peer := job.peer
	caption := job.caption
	replyTo := job.replyToMsgID
	path := job.path
	buildMedia := job.buildMedia
	buildMediaCtx := job.buildMediaCtx

	// The uploader callback runs on the upload goroutine and cannot post tea.Msgs
	// directly, so it forwards (sent,total) over a buffered channel; a pump cmd
	// receives one event at a time and re-arms itself (see handleUploadProgress).
	progressCh := make(chan uploadProgressMsg, 8)
	uploadCmd := func() tea.Msg {
		f, err := client.UploadFile(uploadCtx, internaltg.UploadParams{
			Path: path,
			OnProgress: func(sent, total int64) {
				select {
				case progressCh <- uploadProgressMsg{sentinelID: sentinelID, sent: sent, total: total}:
				default:
				}
			},
		})
		close(progressCh)
		if err != nil {
			return sentMediaConfirmedMsg{chatID: chatID, sentinelID: sentinelID, failed: true}
		}
		var media tg.InputMediaClass
		if buildMediaCtx != nil {
			media, err = buildMediaCtx(uploadCtx, client, f)
			if err != nil {
				return sentMediaConfirmedMsg{chatID: chatID, sentinelID: sentinelID, failed: true}
			}
		} else {
			media = buildMedia(f)
		}
		realID, err := client.SendMedia(uploadCtx, internaltg.SendMediaParams{
			Peer: peer, Media: media, Caption: caption, ReplyToMsgID: replyTo,
		})
		if err != nil {
			return sentMediaConfirmedMsg{chatID: chatID, sentinelID: sentinelID, failed: true}
		}
		return sentMediaConfirmedMsg{chatID: chatID, sentinelID: sentinelID, realID: realID}
	}
	m.uploadProgress[sentinelID] = progressCh
	return m, tea.Batch(uploadCmd, recvProgressCmd(progressCh))
}

// recvProgressCmd waits for the next progress event on ch (or nil when closed).
func recvProgressCmd(ch chan uploadProgressMsg) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return p
	}
}

func (m RootModel) handleUploadProgress(msg uploadProgressMsg) (RootModel, tea.Cmd) {
	if m.st == nil || msg.total == 0 {
		return m, nil
	}
	frac := float64(msg.sent) / float64(msg.total)
	m.st.UpdateLocalMediaProgress(msg.sentinelID, frac)
	m.chat.SetMessagesKeepScroll(m.st.Messages(m.currentChatID))
	// Re-arm the pump to keep receiving until the channel closes.
	if ch, ok := m.uploadProgress[msg.sentinelID]; ok {
		return m, recvProgressCmd(ch)
	}
	return m, nil
}

func (m RootModel) handleSentMediaConfirmed(msg sentMediaConfirmedMsg) (RootModel, tea.Cmd) {
	if m.st == nil {
		return m, nil
	}
	delete(m.uploadCancels, msg.sentinelID)
	delete(m.uploadProgress, msg.sentinelID)
	if msg.failed {
		m.st.MarkLocalMediaFailed(msg.sentinelID)
		if msg.chatID == m.currentChatID {
			m.chat.SetMessagesKeepScroll(m.st.Messages(msg.chatID))
		}
		return m, func() tea.Msg {
			return StatusErrMsg{Text: "media send failed", Sev: components.SeverityWarning}
		}
	}
	m.st.UpdateMessageID(msg.chatID, msg.sentinelID, msg.realID)
	// Keep the local bubble (now at 100%) until the server media is fetched, so
	// the photo never blanks out to a caption-only message in between.
	m.st.UpdateLocalMediaProgress(msg.realID, 1)
	if msg.chatID == m.currentChatID {
		m.chat.SetMessages(m.st.Messages(msg.chatID))
	}
	chat, ok := m.st.GetChat(msg.chatID)
	if !ok || m.tgClient == nil {
		// Cannot refresh; fall back to clearing the placeholder.
		m.st.ClearLocalMedia(msg.realID)
		return m, nil
	}
	return m, refreshSentMediaCmd(m.ctx, m.tgClient, chat.Peer, msg.chatID, msg.realID)
}

// refreshSentMediaCmd re-fetches a just-sent message to obtain the server's media
// refs (the echoed update is suppressed). On error it still reports ok=false so
// the handler can clear the upload placeholder.
func refreshSentMediaCmd(ctx context.Context, client internaltg.Client, peer store.Peer, chatID int64, msgID int) tea.Cmd {
	return func() tea.Msg {
		refreshed, err := client.RefreshMessage(ctx, peer, msgID)
		if err != nil {
			return sentMediaRefreshedMsg{chatID: chatID, msgID: msgID, ok: false}
		}
		return sentMediaRefreshedMsg{chatID: chatID, msgID: msgID, refreshed: refreshed, ok: true}
	}
}

func (m RootModel) handleSentMediaRefreshed(msg sentMediaRefreshedMsg) (RootModel, tea.Cmd) {
	if m.st == nil {
		return m, nil
	}
	if !msg.ok {
		// Refresh failed: drop the placeholder so the caption-only bubble at least
		// stops showing a stuck progress bar.
		m.st.ClearLocalMedia(msg.msgID)
		if msg.chatID == m.currentChatID {
			m.chat.SetMessagesKeepScroll(m.st.Messages(msg.chatID))
		}
		return m, nil
	}
	m.st.AdoptServerMedia(msg.chatID, msg.msgID, msg.refreshed.Photo, msg.refreshed.Document, msg.refreshed.Media)
	if msg.chatID == m.currentChatID {
		m.chat.SetMessagesKeepScroll(m.st.Messages(msg.chatID))
	}
	// Trigger the image download so the photo renders inline.
	for _, mm := range m.st.Messages(msg.chatID) {
		if mm.ID == msg.msgID {
			return m, m.pendingDownloadCmds([]store.Message{mm})
		}
	}
	return m, nil
}

func (m RootModel) handleCancelUpload() (RootModel, tea.Cmd) {
	// The cancel key (x) removes one composer extra at a time, in priority order:
	// a staged attachment first, then an active reply/edit, then an in-flight
	// upload. See item C.
	if m.pendingAttachment != nil {
		m.clearPendingAttachment()
		return m, nil
	}
	if m.chat != nil && (m.chat.ReplyToMsgID() != 0 || m.chat.EditMsgID() != 0) {
		m.chat.ClearPendingAction()
		return m, nil
	}
	if m.chat == nil || m.st == nil {
		return m, nil
	}
	sentinelID := m.chat.SelectedMessageID()
	cancel, ok := m.uploadCancels[sentinelID]
	if !ok {
		return m, nil // not a pending upload
	}
	cancel()
	delete(m.uploadCancels, sentinelID)
	delete(m.uploadProgress, sentinelID)
	m.st.RemoveMessage(m.currentChatID, sentinelID)
	m.chat.RemoveMessage(sentinelID)
	return m, nil
}
