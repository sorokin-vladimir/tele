package ui_test

import (
	"context"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTGClient struct {
	history               []store.Message
	historyErr            error
	sendFunc              func() int
	sendErr               error
	reactionErr           error
	lastReplyToMsgID      int
	downloadPhotoFunc     func() (image.Image, error)
	downloadPhotoFileFunc func(ref store.PhotoRef, dst io.Writer) error
	downloadDocImageFunc  func() (image.Image, error)
	downloadDocFileFunc   func(dst io.Writer) error
	refreshFunc           func(msgID int) (store.Message, error)
	lastSendCtx           context.Context
	sendMediaErr          error
	uploadErr             error
	lastSendMediaParams   internaltg.SendMediaParams
	savedDrafts           []savedDraft
	forwardErr            error
	lastForwardFrom       store.Peer
	lastForwardTo         store.Peer
	lastForwardIDs        []int
	lastSendText          string
	lastSendEntities      []store.MessageEntity
	participants          []store.ChatMember
	sendCount             int
	lastSearchQuery       string
	searchResult          []store.Chat
	searchErr             error
	readReactionsCalls    int
	readMentionsCalls     int
}

type savedDraft struct {
	peerID int64
	text   string
}

func (m *mockTGClient) GetDialogs(_ context.Context) ([]store.Chat, error) { return nil, nil }
func (m *mockTGClient) SearchContacts(_ context.Context, q string, _ int) ([]store.Chat, error) {
	m.lastSearchQuery = q
	return m.searchResult, m.searchErr
}
func (m *mockTGClient) GetDialogFilters(_ context.Context) ([]store.FolderFilter, error) {
	return nil, nil
}
func (m *mockTGClient) GetHistory(_ context.Context, _ store.Peer, _ int, _ int) ([]store.Message, error) {
	if m.historyErr != nil {
		return nil, m.historyErr
	}
	return m.history, nil
}
func (m *mockTGClient) RefreshMessage(_ context.Context, _ store.Peer, msgID int) (store.Message, error) {
	if m.refreshFunc != nil {
		return m.refreshFunc(msgID)
	}
	return store.Message{}, nil
}
func (m *mockTGClient) SendMessage(ctx context.Context, _ store.Peer, text string, replyToMsgID int, entities []store.MessageEntity) (int, error) {
	m.lastSendCtx = ctx
	m.lastReplyToMsgID = replyToMsgID
	m.lastSendText = text
	m.lastSendEntities = entities
	m.sendCount++
	if m.sendErr != nil {
		return 0, m.sendErr
	}
	if m.sendFunc != nil {
		return m.sendFunc(), nil
	}
	return 42, nil
}
func (m *mockTGClient) GetParticipants(_ context.Context, _ store.Peer) ([]store.ChatMember, error) {
	return m.participants, nil
}
func (m *mockTGClient) SendMedia(_ context.Context, p internaltg.SendMediaParams) (int, error) {
	m.lastSendMediaParams = p
	if m.sendMediaErr != nil {
		return 0, m.sendMediaErr
	}
	return 4242, nil
}
func (m *mockTGClient) UploadFile(_ context.Context, p internaltg.UploadParams) (tg.InputFileClass, error) {
	if m.uploadErr != nil {
		return nil, m.uploadErr
	}
	if p.OnProgress != nil {
		p.OnProgress(100, 100)
	}
	return &tg.InputFile{ID: 1, Parts: 1, Name: "a.jpg"}, nil
}
func (m *mockTGClient) MarkRead(_ context.Context, _ store.Peer, _ int) error { return nil }
func (m *mockTGClient) ReadReactions(_ context.Context, _ store.Peer) error {
	m.readReactionsCalls++
	return nil
}
func (m *mockTGClient) ReadMentions(_ context.Context, _ store.Peer) error {
	m.readMentionsCalls++
	return nil
}
func (m *mockTGClient) MarkDialogUnread(_ context.Context, _ store.Peer, _ bool) error {
	return nil
}
func (m *mockTGClient) SetMuted(_ context.Context, _ store.Peer, _ bool) error { return nil }
func (m *mockTGClient) AddToFolder(_ context.Context, _ int, _ store.Peer, _ bool) error {
	return nil
}
func (m *mockTGClient) GetArchivedDialogs(_ context.Context) ([]store.Chat, error) {
	return nil, nil
}
func (m *mockTGClient) SetArchived(_ context.Context, _ store.Peer, _ bool) error { return nil }
func (m *mockTGClient) DownloadPhoto(_ context.Context, _ store.PhotoRef) (image.Image, error) {
	if m.downloadPhotoFunc != nil {
		return m.downloadPhotoFunc()
	}
	return nil, nil
}
func (m *mockTGClient) DownloadPhotoToFile(_ context.Context, ref store.PhotoRef, dst io.Writer) error {
	if m.downloadPhotoFileFunc != nil {
		return m.downloadPhotoFileFunc(ref, dst)
	}
	return nil
}
func (m *mockTGClient) DownloadDocument(_ context.Context, _ store.DocumentRef) ([]byte, error) {
	return nil, nil
}
func (m *mockTGClient) DownloadDocumentToFile(_ context.Context, _ store.DocumentRef, dst io.Writer) error {
	if m.downloadDocFileFunc != nil {
		return m.downloadDocFileFunc(dst)
	}
	return nil
}
func (m *mockTGClient) DownloadDocumentThumb(_ context.Context, _ store.DocumentRef) (image.Image, error) {
	return nil, nil
}
func (m *mockTGClient) DownloadDocumentImage(_ context.Context, _ store.DocumentRef) (image.Image, error) {
	if m.downloadDocImageFunc != nil {
		return m.downloadDocImageFunc()
	}
	return nil, nil
}
func (m *mockTGClient) EditMessage(_ context.Context, _ store.Peer, _ int, _ string, _ []store.MessageEntity) error {
	return nil
}
func (m *mockTGClient) DeleteMessages(_ context.Context, _ store.Peer, _ []int, _ bool) error {
	return nil
}
func (m *mockTGClient) ForwardMessages(_ context.Context, from store.Peer, to store.Peer, ids []int) error {
	m.lastForwardFrom = from
	m.lastForwardTo = to
	m.lastForwardIDs = ids
	return m.forwardErr
}
func (m *mockTGClient) SendReaction(_ context.Context, _ store.Peer, _ int, _ string) error {
	return m.reactionErr
}

func (m *mockTGClient) SetTyping(_ context.Context, _ store.Peer, _ store.TypingAction) error {
	return nil
}
func (m *mockTGClient) SaveDraft(_ context.Context, peer store.Peer, text string) error {
	m.savedDrafts = append(m.savedDrafts, savedDraft{peerID: peer.ID, text: text})
	return nil
}
func (m *mockTGClient) Updates() <-chan store.Event { return make(chan store.Event) }

var _ internaltg.Client = (*mockTGClient)(nil)

func TestRoot_SendFailure_SurfacesErrorAndRemovesSentinel(t *testing.T) {
	mc := &mockTGClient{sendErr: errors.New("offline")}
	m, st := newRootWithOpenChat(t, mc)

	_, cmd := m.Update(screens.SendMsgRequest{Peer: store.Peer{ID: 1, Type: store.PeerUser}, Text: "hi"})
	require.Len(t, st.Messages(1), 1) // optimistic sentinel inserted
	require.NotNil(t, cmd)

	var sawErr bool
	for _, msg := range drainMsgs(cmd()) { // run the SendMessage cmd
		nm, c2 := m.Update(msg)
		m = nm.(ui.RootModel)
		if c2 != nil {
			if _, ok := c2().(ui.StatusErrMsg); ok {
				sawErr = true
			}
		}
	}
	assert.True(t, sawErr, "send failure should surface a StatusErrMsg")
	assert.Empty(t, st.Messages(1), "failed sentinel should be rolled back")
}

type ctxKey struct{}

func TestRoot_ThreadsAppContextIntoCommands(t *testing.T) {
	appCtx := context.WithValue(context.Background(), ctxKey{}, "tele")
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)
	m = m.WithContext(appCtx)

	_, cmd := m.Update(screens.SendMsgRequest{Peer: store.Peer{ID: 1, Type: store.PeerUser}, Text: "hi"})
	require.NotNil(t, cmd)
	cmd() // run the SendMessage cmd

	require.NotNil(t, mock.lastSendCtx)
	assert.Equal(t, "tele", mock.lastSendCtx.Value(ctxKey{}),
		"command should use the app context, not context.Background()")
}

func TestRoot_ReactionFailure_SurfacesError(t *testing.T) {
	mc := &mockTGClient{reactionErr: errors.New("offline")}
	m, st := newRootWithOpenChat(t, mc)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hi", Date: time.Now()})
	m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})

	_, cmd := m.Update(components.ReactConfirmedMsg{Emoji: "👍"})
	require.NotNil(t, cmd)

	failMsg := cmd() // reactionFailedMsg
	_, c2 := m.Update(failMsg)
	require.NotNil(t, c2)
	var sawErr bool
	for _, mm := range drainMsgs(c2()) {
		if _, ok := mm.(ui.StatusErrMsg); ok {
			sawErr = true
		}
	}
	assert.True(t, sawErr, "reaction failure should surface a StatusErrMsg")
}

func TestRoot_EventNewMessage_FiresPhotoDownload(t *testing.T) {
	mc := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mc) // chat ID 1 is the active chat

	newMsg := store.Message{ID: 101, ChatID: 1, Photo: &store.PhotoRef{ID: 9}}
	_, cmd := m.Update(store.Event{Kind: store.EventNewMessage, Message: newMsg})
	require.NotNil(t, cmd) // download command batched
}

func TestRoot_Draft_FlushedToServerOnChatSwitch(t *testing.T) {
	mc := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mc) // chat 1 open
	st.SetChat(store.Chat{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}})

	// Type a draft into chat 1's composer, then switch to chat 2.
	m = m.SetComposerValueForTest("hello Alice")
	_, cmd := m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}}})
	require.NotNil(t, cmd)
	drainMsgs(cmd()) // executes the batched SaveDraft side effect

	var found bool
	for _, d := range mc.savedDrafts {
		if d.peerID == 1 && d.text == "hello Alice" {
			found = true
		}
	}
	assert.True(t, found, "switching chats must persist the outgoing draft; got %+v", mc.savedDrafts)
}

func TestRoot_Draft_EmptyComposerNoServerSave(t *testing.T) {
	mc := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mc) // chat 1 open, composer empty
	st.SetChat(store.Chat{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}})

	// Switch away without typing — no draft change, so no network save.
	_, cmd := m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}}})
	if cmd != nil {
		drainMsgs(cmd())
	}
	assert.Empty(t, mc.savedDrafts, "no draft change must not hit messages.saveDraft")
}

func TestRoot_EventDraftMessage_UpdatesStore(t *testing.T) {
	mc := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mc)

	m.Update(store.Event{Kind: store.EventDraftMessage, ChatID: 1, Draft: "remote draft"})
	got, ok := st.GetChat(1)
	require.True(t, ok)
	assert.Equal(t, "remote draft", got.Draft)
}

func TestPendingDownloadCmds_GIFThumb_FiresDownload(t *testing.T) {
	mc := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mc) // chat ID 1 is the active chat

	gif := store.Message{
		ID: 201, ChatID: 1,
		Media:    &store.MediaRef{Kind: store.MediaGIF},
		Document: &store.DocumentRef{ID: 77, ThumbSize: "m"},
	}
	require.NotNil(t, m.PendingDownloadCmdsForTest([]store.Message{gif}),
		"GIF with a thumb must fire a download")

	noThumb := store.Message{
		ID: 202, ChatID: 1,
		Media:    &store.MediaRef{Kind: store.MediaGIF},
		Document: &store.DocumentRef{ID: 78}, // no ThumbSize
	}
	assert.Nil(t, m.PendingDownloadCmdsForTest([]store.Message{noThumb}),
		"GIF without a thumb must not fire a download")
}

func TestDownloadGifFileCmd_EmitsPathOnSuccess(t *testing.T) {
	mc := &mockTGClient{}
	docID, path, ok := ui.GifFileReadyForTest(mc,
		store.Peer{ID: 1, Type: store.PeerUser}, 10,
		store.DocumentRef{ID: 77, FileName: "anim.mp4"}, t.TempDir())
	require.True(t, ok, "command must produce a gif-file-ready result")
	assert.Equal(t, int64(77), docID)
	assert.NotEmpty(t, path, "downloaded temp path must be set")
}

func TestRoot_HistoryChunk_FiresPhotoDownload(t *testing.T) {
	mc := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mc) // chat ID 1 is the active chat

	older := []store.Message{{ID: 150, ChatID: 1, Photo: &store.PhotoRef{ID: 5}}}
	_, cmd := m.Update(ui.HistoryChunkMsgForTest(1, older))
	require.NotNil(t, cmd)
}

func TestRoot_ChatOpenFailure_ClearsSpinnerAndShowsError(t *testing.T) {
	mc := &mockTGClient{historyErr: errors.New("timeout")}
	st := store.NewMemory()
	chat := store.Chat{ID: 7, Title: "Bob", Peer: store.Peer{ID: 7, Type: store.PeerUser}}
	st.SetChat(chat)
	m := ui.NewRootModel(mc, st, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(ui.RootModel)

	newM, cmd := m.Update(screens.OpenChatMsg{Chat: chat})
	require.NotNil(t, cmd)
	// drain the batched open cmd; one branch yields the chat load error
	m = newM.(ui.RootModel)
	for _, inner := range drainMsgs(cmd()) {
		if inner == nil {
			continue
		}
		nm, _ := m.Update(inner)
		m = nm.(ui.RootModel)
	}
	assert.Contains(t, m.View().Content, "timeout")
}

func TestDownloadPhotoCmd_RefreshesOnExpiredRef(t *testing.T) {
	calls := 0
	mc := &mockTGClient{
		downloadPhotoFunc: func() (image.Image, error) {
			calls++
			if calls == 1 {
				return nil, &tgerr.Error{Code: 400, Type: "FILE_REFERENCE_EXPIRED"}
			}
			return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
		},
		refreshFunc: func(msgID int) (store.Message, error) {
			return store.Message{ID: msgID, ChatID: 7, Photo: &store.PhotoRef{ID: 1, FileReference: []byte("fresh")}}, nil
		},
	}
	cmd := ui.DownloadPhotoCmdForTest(mc, store.Peer{ID: 7, Type: store.PeerUser}, 100, store.PhotoRef{ID: 1})

	msgs := drainMsgs(cmd())
	assert.Equal(t, 2, calls) // retried once after refresh
	var ready *ui.PhotoReadyMsg
	for _, m := range msgs {
		if r, ok := m.(ui.PhotoReadyMsg); ok {
			rr := r
			ready = &rr
		}
	}
	require.NotNil(t, ready)
	assert.NotNil(t, ready.Image)
	assert.Len(t, msgs, 2) // ready image + store-update after refresh
}

func TestDownloadStickerCmd_EmitsPhotoReady(t *testing.T) {
	mc := &mockTGClient{
		downloadDocImageFunc: func() (image.Image, error) {
			return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
		},
	}
	cmd := ui.DownloadStickerCmdForTest(mc, store.Peer{ID: 7, Type: store.PeerUser}, 100, store.DocumentRef{ID: 555, MimeType: "image/webp"})

	msgs := drainMsgs(cmd())
	var ready *ui.PhotoReadyMsg
	for _, m := range msgs {
		if r, ok := m.(ui.PhotoReadyMsg); ok {
			rr := r
			ready = &rr
		}
	}
	require.NotNil(t, ready)
	assert.Equal(t, int64(555), ready.PhotoID)
	assert.NotNil(t, ready.Image)
}

// drainMsgs flattens a (possibly batched) cmd result into its concrete messages.
// newRootOnChat builds a main-screen RootModel focused on a single chat (id 1),
// draining the open-chat command so history loading settles.
func newRootOnChat(t *testing.T, mc *mockTGClient) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	chat := store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}}
	st.SetChat(chat)
	m := ui.NewRootModel(mc, st, 50, false).WithScreen(ui.ScreenMain)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(ui.RootModel)
	nm, cmd := m.Update(screens.OpenChatMsg{Chat: chat})
	m = nm.(ui.RootModel)
	if cmd != nil {
		for _, inner := range drainMsgs(cmd()) {
			if inner == nil {
				continue
			}
			nm2, _ := m.Update(inner)
			m = nm2.(ui.RootModel)
		}
	}
	return m, st
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAttachOpensPickerAndStages(t *testing.T) {
	m, _ := newRootOnChat(t, &mockTGClient{})

	nm, _ := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = nm.(ui.RootModel)
	if !m.FilePickerOpen() {
		t.Fatal("file picker not open after 'u'")
	}

	path := writeTempFile(t, "pic.jpg", "x")
	nm, _ = m.Update(screens.FileSelectedMsg{Path: path})
	m = nm.(ui.RootModel)
	if m.FilePickerOpen() {
		t.Fatal("picker still open after selection")
	}
	if !m.Chat().HasAttachment() {
		t.Fatal("composer chip not set after selection")
	}
}

func TestAttachEntersInsertAndEscKeepsChip(t *testing.T) {
	m, _ := newRootOnChat(t, &mockTGClient{})
	path := writeTempFile(t, "pic.jpg", "x")

	nm, _ := m.Update(screens.FileSelectedMsg{Path: path})
	m = nm.(ui.RootModel)
	// Selecting a file must enter real insert mode (the caption field is active).
	if m.VimMode() != keys.ModeInsert {
		t.Fatalf("after selecting a file mode = %v, want insert", m.VimMode())
	}

	// esc leaves insert mode but keeps the staged attachment.
	nm, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = nm.(ui.RootModel)
	if m.VimMode() != keys.ModeNormal {
		t.Fatalf("after esc mode = %v, want normal", m.VimMode())
	}
	if !m.Chat().HasAttachment() {
		t.Fatal("esc must keep the staged attachment chip")
	}

	// In normal mode, the cancel key drops the staged attachment.
	nm, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = nm.(ui.RootModel)
	if m.Chat().HasAttachment() {
		t.Fatal("'x' must drop the staged attachment")
	}
}

func TestToggleSendAsWorksOnRussianLayout(t *testing.T) {
	m, _ := newRootOnChat(t, &mockTGClient{})
	path := writeTempFile(t, "pic.jpg", "x")
	nm, _ := m.Update(screens.FileSelectedMsg{Path: path})
	m = nm.(ui.RootModel)
	// Staged photo starts as [Photo].
	if !strings.Contains(m.View().Content, "[Photo]") {
		t.Fatalf("expected [Photo] before toggle:\n%s", m.View().Content)
	}
	// ctrl+т on the Russian ЙЦУКЕН layout is reported as ctrl+<Cyrillic е> (the
	// physical 't' key); it must toggle exactly like ctrl+t on a Latin layout.
	nm, _ = m.Update(tea.KeyPressMsg{Code: 'е', Mod: tea.ModCtrl})
	m = nm.(ui.RootModel)
	if !strings.Contains(m.View().Content, "[File]") {
		t.Fatalf("ctrl+<cyrillic e> did not toggle to [File]:\n%s", m.View().Content)
	}
}

func TestAttachPickerIsRendered(t *testing.T) {
	m, _ := newRootOnChat(t, &mockTGClient{})
	nm, _ := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = nm.(ui.RootModel)
	if !m.FilePickerOpen() {
		t.Fatal("file picker not open after 'u'")
	}
	if !strings.Contains(m.View().Content, "filter") {
		t.Fatalf("open file picker is not rendered in the view:\n%s", m.View().Content)
	}
}

func TestSendPhotoOptimisticAndConfirm(t *testing.T) {
	// RefreshMessage supplies the server's media refs after the send confirms.
	mc := &mockTGClient{refreshFunc: func(msgID int) (store.Message, error) {
		return store.Message{
			ID:    msgID,
			Photo: &store.PhotoRef{ID: 9},
			Media: &store.MediaRef{Kind: store.MediaPhoto},
		}, nil
	}}
	m, st := newRootOnChat(t, mc)

	path := writeTempFile(t, "pic.jpg", "hello")
	nm, _ := m.Update(screens.FileSelectedMsg{Path: path})
	m = nm.(ui.RootModel)

	nm, cmd := m.Update(screens.SendMediaRequest{Peer: store.Peer{ID: 1, Type: store.PeerUser}, Caption: "hi"})
	m = nm.(ui.RootModel)

	msgs := st.Messages(1)
	require.NotEmpty(t, msgs)
	last := msgs[len(msgs)-1]
	require.NotNil(t, last.LocalMedia)
	assert.Equal(t, path, last.LocalMedia.Path)
	assert.Equal(t, store.MediaPhoto, last.LocalMedia.Kind)
	assert.True(t, last.IsOut)
	assert.Less(t, last.ID, 0, "optimistic bubble must use a negative sentinel id")

	// Drive the send batch (upload→confirm, progress) and the follow-up refresh
	// command the confirm handler returns.
	require.NotNil(t, cmd)
	var followups []tea.Cmd
	for _, inner := range drainMsgs(cmd()) {
		if inner == nil {
			continue
		}
		nm2, c := m.Update(inner)
		m = nm2.(ui.RootModel)
		if c != nil {
			followups = append(followups, c)
		}
	}
	for _, fc := range followups {
		for _, inner := range drainMsgs(fc()) {
			if inner == nil {
				continue
			}
			nm2, _ := m.Update(inner)
			m = nm2.(ui.RootModel)
		}
	}

	msgs = st.Messages(1)
	last = msgs[len(msgs)-1]
	assert.Equal(t, 4242, last.ID, "sentinel must be swapped to the real id")
	assert.Nil(t, last.LocalMedia, "LocalMedia must be cleared after server media adopted")
	require.NotNil(t, last.Photo, "server photo ref must be adopted so it renders without manual refresh")
	assert.Equal(t, int64(9), last.Photo.ID)
}

func TestSendPhotoFailureMarksFailed(t *testing.T) {
	mc := &mockTGClient{sendMediaErr: errors.New("boom")}
	m, st := newRootOnChat(t, mc)

	path := writeTempFile(t, "a.jpg", "x")
	nm, _ := m.Update(screens.FileSelectedMsg{Path: path})
	m = nm.(ui.RootModel)

	nm, cmd := m.Update(screens.SendMediaRequest{Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m = nm.(ui.RootModel)
	require.NotNil(t, cmd)
	for _, inner := range drainMsgs(cmd()) {
		if inner == nil {
			continue
		}
		nm2, _ := m.Update(inner)
		m = nm2.(ui.RootModel)
	}

	last := st.Messages(1)[0]
	require.NotNil(t, last.LocalMedia)
	assert.Equal(t, store.UploadFailed, last.LocalMedia.UploadState)
}

func TestSendDocumentOptimisticAndConfirm(t *testing.T) {
	// RefreshMessage supplies the server's document ref after the send confirms.
	mc := &mockTGClient{refreshFunc: func(msgID int) (store.Message, error) {
		return store.Message{
			ID:       msgID,
			Document: &store.DocumentRef{ID: 9, FileName: "report.pdf", Size: 4096},
			Media:    &store.MediaRef{Kind: store.MediaFile, FileName: "report.pdf", Size: 4096},
		}, nil
	}}
	m, st := newRootOnChat(t, mc)

	// A .pdf resolves (by extension) to application/pdf -> MediaFile default.
	path := writeTempFile(t, "report.pdf", "%PDF-1.4 hello")
	nm, _ := m.Update(screens.FileSelectedMsg{Path: path})
	m = nm.(ui.RootModel)

	nm, cmd := m.Update(screens.SendMediaRequest{Peer: store.Peer{ID: 1, Type: store.PeerUser}, Caption: "doc"})
	m = nm.(ui.RootModel)

	msgs := st.Messages(1)
	require.NotEmpty(t, msgs)
	last := msgs[len(msgs)-1]
	require.NotNil(t, last.LocalMedia)
	assert.Equal(t, store.MediaFile, last.LocalMedia.Kind, "document bubble must carry MediaFile kind")
	assert.Less(t, last.ID, 0, "optimistic bubble must use a negative sentinel id")

	// Drive the upload->send->confirm batch and the follow-up refresh command.
	require.NotNil(t, cmd)
	var followups []tea.Cmd
	for _, inner := range drainMsgs(cmd()) {
		if inner == nil {
			continue
		}
		nm2, c := m.Update(inner)
		m = nm2.(ui.RootModel)
		if c != nil {
			followups = append(followups, c)
		}
	}
	for _, fc := range followups {
		for _, inner := range drainMsgs(fc()) {
			if inner == nil {
				continue
			}
			nm2, _ := m.Update(inner)
			m = nm2.(ui.RootModel)
		}
	}

	// The generic (forced) document path must have been used.
	doc, ok := mc.lastSendMediaParams.Media.(*tg.InputMediaUploadedDocument)
	require.True(t, ok, "got %T, want *tg.InputMediaUploadedDocument", mc.lastSendMediaParams.Media)
	assert.True(t, doc.ForceFile)

	msgs = st.Messages(1)
	last = msgs[len(msgs)-1]
	assert.Equal(t, 4242, last.ID, "sentinel must be swapped to the real id")
	assert.Nil(t, last.LocalMedia, "LocalMedia must be cleared after server media adopted")
	require.NotNil(t, last.Document, "server document ref must be adopted")
	assert.Equal(t, "report.pdf", last.Document.FileName)
}

func TestSendMedia_Video_SendsInlineVideoDocument(t *testing.T) {
	mc := &mockTGClient{refreshFunc: func(msgID int) (store.Message, error) {
		return store.Message{
			ID:       msgID,
			Document: &store.DocumentRef{ID: 9, FileName: "clip.mp4", ThumbSize: "m"},
			Media:    &store.MediaRef{Kind: store.MediaVideo, FileName: "clip.mp4"},
		}, nil
	}}
	m, st := newRootOnChat(t, mc)

	// A .mp4 resolves (by extension) to video/mp4 -> MediaVideo default.
	path := writeTempFile(t, "clip.mp4", "\x00\x00\x00\x18ftypmp42 fake")
	nm, _ := m.Update(screens.FileSelectedMsg{Path: path})
	m = nm.(ui.RootModel)

	nm, cmd := m.Update(screens.SendMediaRequest{Peer: store.Peer{ID: 1, Type: store.PeerUser}, Caption: "vid"})
	m = nm.(ui.RootModel)

	msgs := st.Messages(1)
	require.NotEmpty(t, msgs)
	last := msgs[len(msgs)-1]
	require.NotNil(t, last.LocalMedia)
	assert.Equal(t, store.MediaVideo, last.LocalMedia.Kind, "video bubble must carry MediaVideo kind")
	assert.Less(t, last.ID, 0, "optimistic bubble must use a negative sentinel id")

	// Drive the upload->send->confirm batch and the follow-up refresh command.
	require.NotNil(t, cmd)
	var followups []tea.Cmd
	for _, inner := range drainMsgs(cmd()) {
		if inner == nil {
			continue
		}
		nm2, c := m.Update(inner)
		m = nm2.(ui.RootModel)
		if c != nil {
			followups = append(followups, c)
		}
	}
	for _, fc := range followups {
		for _, inner := range drainMsgs(fc()) {
			if inner == nil {
				continue
			}
			nm2, _ := m.Update(inner)
			m = nm2.(ui.RootModel)
		}
	}

	// The inline-video document path must have been used: a video document
	// (NOT ForceFile) carrying DocumentAttributeVideo with SupportsStreaming.
	doc, ok := mc.lastSendMediaParams.Media.(*tg.InputMediaUploadedDocument)
	require.True(t, ok, "got %T, want *tg.InputMediaUploadedDocument", mc.lastSendMediaParams.Media)
	assert.False(t, doc.ForceFile, "video must not force the generic file path")
	var hasVideoAttr bool
	for _, a := range doc.Attributes {
		if v, ok := a.(*tg.DocumentAttributeVideo); ok && v.SupportsStreaming {
			hasVideoAttr = true
		}
	}
	assert.True(t, hasVideoAttr, "must carry a streaming DocumentAttributeVideo")

	msgs = st.Messages(1)
	last = msgs[len(msgs)-1]
	assert.Equal(t, 4242, last.ID, "sentinel must be swapped to the real id")
	require.NotNil(t, last.Document, "server document ref must be adopted")
	assert.Equal(t, "clip.mp4", last.Document.FileName)
}

func drainMsgs(msg tea.Msg) []tea.Msg {
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, c())
	}
	return out
}

func TestRoot_StatusErrMsg_SetsAndSchedulesClear(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	newM, cmd := newM.(ui.RootModel).Update(ui.StatusErrMsg{Text: "network down", Sev: components.SeverityError})
	root := newM.(ui.RootModel)
	root.SettleToastsForTest()
	assert.Contains(t, root.View().Content, "network down")
	require.NotNil(t, cmd) // an auto-clear tick was scheduled
}

func TestRoot_ClearStatusErrMsg_StaleSerialKeepsError(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m2, _ := newM.(ui.RootModel).Update(ui.StatusErrMsg{Text: "first", Sev: components.SeverityError})
	root := m2.(ui.RootModel)
	m3, _ := root.Update(ui.ClearStatusErrMsg{Serial: -999}) // never a real serial
	settled := m3.(ui.RootModel)
	settled.SettleToastsForTest()
	assert.Contains(t, settled.View().Content, "first")
}

// An error completion clears the download indicator and surfaces the error text.
func TestRoot_DocumentOpenDone_ErrorShowsStatus(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	done := ui.DocumentOpenDoneMsgForTest(1, "open file failed: boom", components.SeverityWarning)
	m2, _ := newM.(ui.RootModel).Update(done)
	root := m2.(ui.RootModel)
	root.SettleToastsForTest()
	assert.Contains(t, root.View().Content, "open file failed: boom")
}

// A successful completion adds no error text to the status bar.
func TestRoot_DocumentOpenDone_SuccessNoError(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	done := ui.DocumentOpenDoneMsgForTest(1, "", components.SeverityWarning)
	m2, _ := newM.(ui.RootModel).Update(done)
	assert.NotContains(t, m2.(ui.RootModel).View().Content, "failed")
}

func TestRoot_FileDownloadDone_SuccessShowsPath(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	done := ui.FileDownloadDoneMsgForTest(1, "Saved to /tmp/report.pdf", components.SeverityInfo)
	m2, _ := newM.(ui.RootModel).Update(done)
	root := m2.(ui.RootModel)
	root.SettleToastsForTest()
	assert.Contains(t, root.View().Content, "Saved to /tmp/report.pdf")
}

func TestRoot_FileDownloadDone_ErrorShowsText(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	done := ui.FileDownloadDoneMsgForTest(1, "download failed: boom", components.SeverityWarning)
	m2, _ := newM.(ui.RootModel).Update(done)
	root := m2.(ui.RootModel)
	root.SettleToastsForTest()
	assert.Contains(t, root.View().Content, "download failed: boom")
}

// Selecting a file and pressing the download key starts a download (dispatches a
// command) without launching an external app.
func TestRoot_DownloadKey_StartsFileDownload(t *testing.T) {
	fileMsg := store.Message{
		ID: 7, ChatID: 1, Date: time.Now(),
		Media:    &store.MediaRef{Kind: store.MediaFile},
		Document: &store.DocumentRef{ID: 5, FileName: "report.pdf"},
	}
	m, _ := newRootOnChat(t, &mockTGClient{history: []store.Message{fileMsg}})
	_ = m.View() // establish the selected message

	m2, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	// The status-bar download indicator becomes active for the file's name.
	assert.Contains(t, m2.(ui.RootModel).View().Content, "downloading report.pdf")
}

func TestRoot_InitialScreen_Login(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	assert.Equal(t, ui.ScreenLogin, m.CurrentScreen())
}

func TestRoot_InitialChatList_IsFocused(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	assert.True(t, m.ChatList().Focused(), "chatList must be focused from the start so cursor highlight is visible")
}

func TestRoot_2_FocusesChat(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
	newM, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChat, root.CurrentFocus())
}

func TestRoot_1_FocusesChatList(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m = m.WithFocus(ui.FocusChat)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChatList, root.CurrentFocus())
}

func TestRoot_TransitionToMain(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.ScreenMain, root.CurrentScreen())
}

func TestRoot_CtrlC_Quits(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	// Settle the animation loops so the returned command is the quit alone, not
	// batched with an animation re-arm (issue #147).
	m.ChatList().SetChats([]store.Chat{{ID: 1}})
	m.Chat().SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	assert.NotNil(t, cmd)
	msg := cmd()
	_, isQuit := msg.(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestRoot_LoadMoreMsg_DispatchesGetHistory(t *testing.T) {
	mock := &mockTGClient{}
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(mock, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// Set current chat to 1 by sending OpenChatMsg
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	m = newM.(ui.RootModel)

	newM, cmd := m.Update(screens.LoadMoreMsg{ChatID: 1, OffsetID: 5})
	_ = newM
	require.NotNil(t, cmd)
	// cmd should trigger a GetHistory call — verify it returns a non-nil message
	result := cmd()
	assert.NotNil(t, result)
}

func TestRoot_LoadMore_GuardsConcurrentRequests(t *testing.T) {
	mock := &mockTGClient{history: []store.Message{{ID: 1, ChatID: 1, Date: time.Now()}}}
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(mock, st, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	m = newM.(ui.RootModel)

	// First load-older dispatches a fetch and arms the in-flight guard.
	newM, cmd1 := m.Update(screens.LoadMoreMsg{ChatID: 1, OffsetID: 5})
	m = newM.(ui.RootModel)
	require.NotNil(t, cmd1)

	// A second load-older while the first is still in flight is ignored — no
	// duplicate fetch that would later stack a duplicate chunk (issue #120).
	newM, cmd2 := m.Update(screens.LoadMoreMsg{ChatID: 1, OffsetID: 5})
	m = newM.(ui.RootModel)
	assert.Nil(t, cmd2)

	// Once the chunk resolves the guard clears, so further loads are allowed.
	newM, _ = m.Update(cmd1())
	m = newM.(ui.RootModel)
	_, cmd3 := m.Update(screens.LoadMoreMsg{ChatID: 1, OffsetID: 1})
	assert.NotNil(t, cmd3)
}

func TestRoot_SlashKey_ActivatesSearch(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	root := newM.(ui.RootModel)
	assert.True(t, root.SearchActive())
}

func TestRoot_CloseSearchMsg_DeactivatesSearch(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = newM.(ui.RootModel)
	require.True(t, m.SearchActive())
	newM, _ = m.Update(screens.CloseSearchMsg{})
	m = newM.(ui.RootModel)
	assert.False(t, m.SearchActive())
}

func TestRoot_SearchOpenChatMsg_ClosesSearch(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1}})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = newM.(ui.RootModel)
	newM, _ = m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 1, Title: "Alice"}})
	m = newM.(ui.RootModel)
	assert.False(t, m.SearchActive())
}

func newRootWithTwoChats(t *testing.T) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	st.SetChat(store.Chat{ID: 2, Title: "Bob"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	return newM.(ui.RootModel), st
}

func TestRoot_NewMessageEvent_UpdatesChatList(t *testing.T) {
	m, _ := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 2, Text: "hi", Date: time.Now()},
	}
	newM, _ := m.Update(evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Chats()
	require.Len(t, chats, 2)
	assert.Equal(t, int64(2), chats[0].ID, "chat 2 should bubble to top after new message")
}

func TestRoot_NewMessageEvent_IncrementsUnread(t *testing.T) {
	m, _ := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ID: 1, ChatID: 2, Text: "hi"},
	}
	newM, _ := m.Update(evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Chats()
	var chat2 store.Chat
	for _, c := range chats {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 1, chat2.UnreadCount)
}

func TestRoot_NewMessageEvent_UnreadPersistsAcrossMultipleEvents(t *testing.T) {
	m, _ := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ID: 1, ChatID: 2, Text: "first"},
	}
	newM, _ := m.Update(evt)
	m = newM.(ui.RootModel)

	evt2 := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ID: 2, ChatID: 2, Text: "second"},
	}
	newM, _ = m.Update(evt2)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Chats()
	var chat2 store.Chat
	for _, c := range chats {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 2, chat2.UnreadCount, "unread count should accumulate across multiple new-message events")
}

func TestRoot_NewMessageEvent_NoUnreadForCurrentChat(t *testing.T) {
	m, _ := newRootWithTwoChats(t)

	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 1, Title: "Alice"}})
	m = newM.(ui.RootModel)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 1, Text: "hi"},
	}
	newM, _ = m.Update(evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Chats()
	var chat1 store.Chat
	for _, c := range chats {
		if c.ID == 1 {
			chat1 = c
		}
	}
	assert.Equal(t, 0, chat1.UnreadCount)
}

func TestRoot_NewMessageEvent_NoUnreadForOutgoingMessage(t *testing.T) {
	m, _ := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 2, Text: "sent from phone", IsOut: true},
	}
	newM, _ := m.Update(evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Chats()
	var chat2 store.Chat
	for _, c := range chats {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 0, chat2.UnreadCount)
}

func newRootWithOpenChat(t *testing.T, mock *mockTGClient) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(mock, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	return newM.(ui.RootModel), st
}

func TestRoot_Send_ShowsSentinelImmediately(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)

	_, _ = m.Update(screens.SendMsgRequest{
		Peer: store.Peer{ID: 1, Type: store.PeerUser},
		Text: "hello",
	})

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Less(t, msgs[0].ID, 0, "sentinel should have a negative ID")
	assert.Equal(t, "hello", msgs[0].Text)
	assert.True(t, msgs[0].IsOut)
}

func TestRoot_Send_ConfirmationReplacesSentinel(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)

	newM, cmd := m.Update(screens.SendMsgRequest{
		Peer: store.Peer{ID: 1, Type: store.PeerUser},
		Text: "hello",
	})
	m = newM.(ui.RootModel)
	require.NotNil(t, cmd)

	confirmMsg := cmd()
	newM, _ = m.Update(confirmMsg)
	_ = newM

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Equal(t, 42, msgs[0].ID, "sentinel should be replaced with real ID")
}

func TestRoot_Send_FailedSendRemovesSentinel(t *testing.T) {
	mock := &mockTGClient{sendFunc: func() int { return 0 }}
	m, st := newRootWithOpenChat(t, mock)

	newM, cmd := m.Update(screens.SendMsgRequest{
		Peer: store.Peer{ID: 1, Type: store.PeerUser},
		Text: "hello",
	})
	m = newM.(ui.RootModel)
	require.NotNil(t, cmd)

	confirmMsg := cmd()
	newM, _ = m.Update(confirmMsg)
	_ = newM

	msgs := st.Messages(1)
	assert.Empty(t, msgs, "sentinel should be removed when send fails")
}

func TestRootModel_PhotoDownloadDispatchedOnHistory(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)
	m2, cmd := m.Update(ui.ChatHistoryMsg{
		ChatID: 1,
		Messages: []store.Message{
			{ID: 10, ChatID: 1, Text: "hello"},
			{ID: 11, ChatID: 1, Photo: &store.PhotoRef{ID: 77, ThumbSize: "m"}},
		},
	})
	_ = m2
	require.NotNil(t, cmd, "should return cmd (download + markread) for messages with photo")
}

func TestRootModel_PhotoReadyMsg_StoresImage(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	m2, _ := m.Update(ui.PhotoReadyMsg{PhotoID: 55, Image: img})
	_ = m2
	// No panic — image cache updated without crashing
}

func TestRoot_Send_ConcurrentSentinelsHaveDistinctIDs(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)

	// Send first message
	newM, _ := m.Update(screens.SendMsgRequest{
		Peer: store.Peer{ID: 1, Type: store.PeerUser},
		Text: "first",
	})
	m = newM.(ui.RootModel)

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	id1 := msgs[0].ID
	assert.Less(t, id1, 0, "first sentinel should have a negative ID")

	// Send second message without running any cmds
	newM, _ = m.Update(screens.SendMsgRequest{
		Peer: store.Peer{ID: 1, Type: store.PeerUser},
		Text: "second",
	})
	_ = newM

	msgs = st.Messages(1)
	require.Len(t, msgs, 2)

	id2 := msgs[1].ID
	assert.Less(t, id2, 0, "second sentinel should have a negative ID")
	assert.NotEqual(t, id1, id2, "two sentinel messages must have distinct IDs")
}

func TestRoot_Space_OpensContextMenu(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)

	assert.True(t, m.ContextMenuOpen())
}

func TestRoot_Space_NoMenuWhenNoMessages(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)
	// No messages added — SelectedMessageID() returns 0

	newM, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)

	assert.False(t, m.ContextMenuOpen(), "menu should not open when no message is selected")
}

func TestRoot_EscKeepsReply_XClearsIt(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hi", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	// reply -> enters insert mode with the reply set
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = newM.(ui.RootModel)
	require.Equal(t, 10, m.Chat().ReplyToMsgID())

	// esc -> back to normal mode, reply kept (only unfocus)
	newM, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = newM.(ui.RootModel)
	require.Equal(t, 10, m.Chat().ReplyToMsgID(), "esc must keep the reply")

	// x -> explicitly clears the reply
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = newM.(ui.RootModel)
	assert.Equal(t, 0, m.Chat().ReplyToMsgID(), "x must clear the active reply")
}

func TestRoot_EscKeepsEdit_XClearsIt(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 11, ChatID: 1, Text: "mine", IsOut: true, Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	// edit -> enters insert mode with the edit set
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = newM.(ui.RootModel)
	require.Equal(t, 11, m.Chat().EditMsgID())

	// esc -> back to normal mode, edit kept
	newM, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = newM.(ui.RootModel)
	require.Equal(t, 11, m.Chat().EditMsgID(), "esc must keep the edit")

	// x -> explicitly clears the edit
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = newM.(ui.RootModel)
	assert.Equal(t, 0, m.Chat().EditMsgID(), "x must clear the active edit")
}

func TestRoot_ReactKey_OpensReactionPicker(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)
	require.False(t, m.ReactionPickerOpen())

	newM, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = newM.(ui.RootModel)
	assert.True(t, m.ReactionPickerOpen(), "react key should open the reaction picker")
}

func TestRoot_ForwardKey_OpensPicker(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)
	require.False(t, m.SearchActive())

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = newM.(ui.RootModel)
	assert.True(t, m.SearchActive(), "forward key should open the chat picker")
}

func TestRoot_ForwardToChat_CallsClient(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)
	target := store.Peer{ID: 999, Type: store.PeerUser, AccessHash: 7}

	newM, cmd := m.Update(screens.ForwardToChatRequest{ToPeer: target, MsgID: 5})
	m = newM.(ui.RootModel)
	require.False(t, m.SearchActive(), "picker should close on confirm")
	require.NotNil(t, cmd)
	_ = cmd() // run the managed Cmd performing the RPC

	assert.Equal(t, target, mock.lastForwardTo)
	assert.Equal(t, []int{5}, mock.lastForwardIDs)
	assert.Equal(t, int64(1), mock.lastForwardFrom.ID, "source peer is the open chat")
}

func TestRoot_ForwardWithComment_SendsCommentThenForwards(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)
	target := store.Peer{ID: 999, Type: store.PeerUser, AccessHash: 7}

	_, cmd := m.Update(screens.ForwardToChatRequest{ToPeer: target, MsgID: 5, Comment: "look at this"})
	require.NotNil(t, cmd)
	_ = cmd() // run the managed Cmd

	assert.Equal(t, 1, mock.sendCount, "comment must be sent")
	assert.Equal(t, "look at this", mock.lastSendText)
	assert.Equal(t, target, mock.lastForwardTo)
	assert.Equal(t, []int{5}, mock.lastForwardIDs)
}

func TestRoot_ForwardWithoutComment_DoesNotSend(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)
	target := store.Peer{ID: 999, Type: store.PeerUser}

	_, cmd := m.Update(screens.ForwardToChatRequest{ToPeer: target, MsgID: 5})
	require.NotNil(t, cmd)
	_ = cmd()

	assert.Equal(t, 0, mock.sendCount, "no comment means no extra message")
	assert.Equal(t, []int{5}, mock.lastForwardIDs)
}

func TestRoot_Forward_BumpsTargetChatToTop(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock) // chat 1 is open/current
	st.SetChat(store.Chat{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}})
	// Source message lives in the open chat; chat 1 has an older last message.
	st.AppendMessage(store.Message{ID: 7, ChatID: 1, Text: "src", Date: time.Now().Add(-time.Hour)})

	target := store.Peer{ID: 2, Type: store.PeerUser}
	_, cmd := m.Update(screens.ForwardToChatRequest{ToPeer: target, MsgID: 7})
	require.NotNil(t, cmd)
	done := cmd()           // run the RPC cmd -> forwardDoneMsg
	m2, _ := m.Update(done) // handleForwardDone bumps the target chat
	_ = m2.(ui.RootModel)

	got, ok := st.GetChat(2)
	require.True(t, ok)
	require.NotNil(t, got.LastMessage, "target chat should have a last message after forward")
	assert.True(t, got.LastMessage.IsOut)
	assert.Equal(t, int64(2), st.Chats()[0].ID, "forward target bubbles to the top of the list")
}

func TestRoot_ForwardRestricted_ShowsStatus(t *testing.T) {
	mock := &mockTGClient{forwardErr: internaltg.ErrForwardRestricted}
	m, _ := newRootWithOpenChat(t, mock)
	target := store.Peer{ID: 999, Type: store.PeerUser}

	_, cmd := m.Update(screens.ForwardToChatRequest{ToPeer: target, MsgID: 5})
	require.NotNil(t, cmd)
	done := cmd()
	_, cmd2 := m.Update(done)
	require.NotNil(t, cmd2)
	se, ok := cmd2().(ui.StatusErrMsg)
	require.True(t, ok, "restricted forward should surface a StatusErrMsg")
	assert.Contains(t, se.Text, "restricted")
}

func TestRoot_ContextMenu_EscCloses(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	// open menu
	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())

	// close with esc
	newM, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = newM.(ui.RootModel)

	// dispatch the CloseContextMenuMsg cmd if present
	require.NotNil(t, cmd, "esc should return a CloseContextMenuMsg cmd")
	newM, _ = m.Update(cmd())
	m = newM.(ui.RootModel)

	assert.False(t, m.ContextMenuOpen())
}

func TestWithKeyMap_RebindOpensContextMenu(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"chat": {"open_context_menu": {"m"}},
	})
	require.Empty(t, warns)
	m = m.WithKeyMap(km)

	// "m" now opens the context menu (was "space").
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = newM.(ui.RootModel)
	assert.True(t, m.ContextMenuOpen())
}

func TestRoot_DeleteMsgRequest_RemovesFromStore(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	require.Len(t, st.Messages(1), 1)

	newM, _ = m.Update(components.DeleteMsgRequest{MsgID: 10, Revoke: false})
	_ = newM

	assert.Empty(t, st.Messages(1), "message removed from store")
}

func TestRoot_ContextMenu_QuitKeyDoesNotQuit(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	// open context menu
	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())

	// q while menu is open must not close the app
	newM, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = newM.(ui.RootModel)

	assert.True(t, m.ContextMenuOpen(), "context menu must stay open after q")
	assert.Nil(t, cmd, "q while menu is open must not produce a quit cmd")
}

func TestRoot_ReplyMsgRequest_ClosesMenuAndFocusesComposer(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original", SenderName: "Alice", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	// open context menu first
	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())

	newM, _ = m.Update(components.ReplyMsgRequest{MsgID: 10})
	m = newM.(ui.RootModel)

	assert.False(t, m.ContextMenuOpen(), "context menu must close after ReplyMsgRequest")
	assert.True(t, m.Chat().ComposerFocused(), "composer must be focused after ReplyMsgRequest")
	assert.Equal(t, keys.ModeInsert, m.VimMode(), "ReplyMsgRequest must switch root to insert mode")
}

func TestRoot_Send_WithReply_PassesReplyToMsgID(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	_, cmd := m.Update(screens.SendMsgRequest{
		Peer:         store.Peer{ID: 1, Type: store.PeerUser},
		Text:         "my reply",
		ReplyToMsgID: 10,
	})
	require.NotNil(t, cmd)
	cmd() // triggers mock.SendMessage

	assert.Equal(t, 10, mock.lastReplyToMsgID)
}

func TestRoot_R_Key_ActivatesReplyMode(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original", SenderName: "Alice", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = newM.(ui.RootModel)

	assert.True(t, m.Chat().ComposerFocused(), "r key must activate reply mode and focus composer")
	assert.Equal(t, 10, m.Chat().ReplyToMsgID(), "r key must set reply target")
	assert.Equal(t, keys.ModeInsert, m.VimMode(), "r key must switch root to insert mode")
}

func TestRoot_OpenChat_ClearsPendingReply(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original", SenderName: "Alice", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	// activate reply mode
	newM, _ = m.Update(components.ReplyMsgRequest{MsgID: 10})
	m = newM.(ui.RootModel)
	require.Equal(t, 10, m.Chat().ReplyToMsgID(), "reply must be active before switching chat")

	// switch to a different chat
	st.SetChat(store.Chat{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}})
	newM, _ = m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}}})
	m = newM.(ui.RootModel)

	assert.Equal(t, 0, m.Chat().ReplyToMsgID(), "switching chat must clear pending reply")
}

func TestRoot_Send_SentinelCarriesReplyToMsgID(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)

	_, _ = m.Update(screens.SendMsgRequest{
		Peer:         store.Peer{ID: 1, Type: store.PeerUser},
		Text:         "my reply",
		ReplyToMsgID: 10,
	})

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Equal(t, 10, msgs[0].ReplyToMsgID, "sentinel must carry ReplyToMsgID")
}

func TestRoot_h_CyclesFocusLeft(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m = m.WithFocus(ui.FocusChat)
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChatList, root.CurrentFocus())
}

func TestRoot_l_CyclesFocusRight(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChat, root.CurrentFocus())
}

func TestRoot_FolderSelectedMsg_FiltersChatList(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}, IsContact: true})
	st.SetChat(store.Chat{ID: 2, Title: "Group", Peer: store.Peer{ID: 2, Type: store.PeerGroup}})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	filter := store.FolderFilter{ID: 1, Title: "Contacts", Contacts: true}
	newM, _ := m.Update(ui.FolderFiltersMsg{Filters: []store.FolderFilter{filter}})
	m = newM.(ui.RootModel)

	// Select the Contacts folder
	selectedFilter := filter
	newM, _ = m.Update(screens.FolderSelectedMsg{Filter: &selectedFilter})
	root := newM.(ui.RootModel)

	// Only the contact chat should be in the chatlist
	chats := root.ChatList().Chats()
	require.Len(t, chats, 1)
	assert.Equal(t, int64(1), chats[0].ID)
}

func TestRoot_FolderFiltersMsg_SetsFolders(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	filters := []store.FolderFilter{{ID: 1, Title: "Work"}}
	newM, _ := m.Update(ui.FolderFiltersMsg{Filters: filters})
	root := newM.(ui.RootModel)
	assert.True(t, root.HasFolders())
}

func TestRoot_0_FocusesFolders(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	filters := []store.FolderFilter{{ID: 1, Title: "Work"}}
	m2, _ := m.Update(ui.FolderFiltersMsg{Filters: filters})
	root := m2.(ui.RootModel)
	newM, _ := root.Update(tea.KeyPressMsg{Code: '0', Text: "0"})
	root2 := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusFolders, root2.CurrentFocus())
}

func TestRoot_FocusNext_DoesNotAutoOpenChat(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	st.SetChat(store.Chat{ID: 2, Title: "Bob"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusChatList, m.CurrentFocus())

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = newM.(ui.RootModel)
	newM, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChat, m.CurrentFocus())
	assert.Nil(t, cmd, "switching focus must not open a chat")
}

func TestRoot_FolderSelectedMsg_FocusesChatList(t *testing.T) {
	st := store.NewMemory()
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	filters := []store.FolderFilter{{ID: 1, Title: "Work"}}
	newM, _ := m.Update(ui.FolderFiltersMsg{Filters: filters})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: '0', Text: "0"})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusFolders, m.CurrentFocus())

	filter := store.FolderFilter{ID: 1, Title: "Work"}
	newM, _ = m.Update(screens.FolderSelectedMsg{Filter: &filter})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
}

func TestRoot_OpenSameChatAgain_OnlyFocusesChatPane(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)

	newM, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusChatList, m.CurrentFocus())

	newM, cmd := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChat, m.CurrentFocus())
	assert.Nil(t, cmd, "re-opening same chat must not trigger a history reload")
}

func TestRoot_EventDeleteMessages_Channel_RemovesFromCurrentChat(t *testing.T) {
	m, st := newRootWithTwoChats(t)
	now := time.Now()
	st.SetMessages(1, []store.Message{
		{ID: 10, ChatID: 1, Text: "hello", Date: now},
		{ID: 11, ChatID: 1, Text: "world", Date: now},
	})
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 1, Title: "Alice"}})
	m = newM.(ui.RootModel)

	evt := store.Event{
		Kind:   store.EventDeleteMessages,
		ChatID: 1,
		MsgIDs: []int{10},
	}
	newM, _ = m.Update(evt)
	_ = newM.(ui.RootModel)

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Equal(t, 11, msgs[0].ID)
}

func TestRoot_EventDeleteMessages_NonChannel_TargetsOwningChat(t *testing.T) {
	m, st := newRootWithTwoChats(t)
	now := time.Now()
	// In the shared pts box message IDs are globally unique, so a delete-without-
	// chatID resolves to exactly one chat via the store index (issue #72).
	st.SetMessages(1, []store.Message{{ID: 5, ChatID: 1, Text: "a", Date: now}})
	st.SetMessages(2, []store.Message{{ID: 6, ChatID: 2, Text: "b", Date: now}})

	evt := store.Event{
		Kind:   store.EventDeleteMessages,
		ChatID: 0,
		MsgIDs: []int{5},
	}
	newM, _ := m.Update(evt)
	_ = newM.(ui.RootModel)

	assert.Empty(t, st.Messages(1))   // owning chat lost the message
	require.Len(t, st.Messages(2), 1) // unrelated chat untouched
}

func TestRoot_ContextMenu_PhotoMessage_ShowsAllThreeActions(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(ui.RootModel)
	st.AppendMessage(store.Message{
		ID:     10,
		ChatID: 1,
		Text:   "photo msg",
		Photo:  &store.PhotoRef{ID: 77},
		Date:   time.Now(),
	})
	newM, _ = m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())
	content := xansi.Strip(m.View().Content)
	assert.Contains(t, content, "open photo")
	assert.Contains(t, content, "Open photo externally")
	assert.Contains(t, content, "save photo (download)")
}

func TestRoot_ContextMenu_NonMediaMessage_HidesMediaActions(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(ui.RootModel)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "text msg", Date: time.Now()})
	newM, _ = m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())
	content := m.View().Content
	assert.NotContains(t, content, "Open externally")
	assert.NotContains(t, content, "Download")
}

func TestRoot_EventUserPresence_UpdatesChatOnline(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m.ChatList().SetChats(st.Chats())

	newM, _ := m.Update(store.Event{
		Kind:   store.EventUserPresence,
		ChatID: 1,
		Online: true,
	})
	_ = newM.(ui.RootModel)

	chat, ok := st.GetChat(1)
	require.True(t, ok)
	assert.True(t, chat.Online)
}

func TestRoot_EventUserPresence_NoopWhenOnlineUnchanged(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}, Online: true})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m.ChatList().SetChats(st.Chats())
	// Open a chat so the idle logo is hidden; otherwise its tick loop re-arms and
	// the no-op event would appear to return a command (issue #147).
	m.Chat().SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})

	newM, cmd := m.Update(store.Event{
		Kind:   store.EventUserPresence,
		ChatID: 1,
		Online: true, // same as stored
	})
	_ = newM.(ui.RootModel)

	assert.Nil(t, cmd)
	chat, ok := st.GetChat(1)
	require.True(t, ok)
	assert.True(t, chat.Online)
}

func TestRoot_EventMuteUpdate_UpdatesStoreMuteFlag(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m.ChatList().SetChats(st.Chats())

	newM, _ := m.Update(store.Event{
		Kind:   store.EventMuteUpdate,
		ChatID: 1,
		Muted:  true,
	})
	_ = newM.(ui.RootModel)

	chat, ok := st.GetChat(1)
	require.True(t, ok)
	assert.True(t, chat.IsMuted)
}

func TestRoot_EventMuteUpdate_NoopWhenUnchanged(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}, IsMuted: true})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m.ChatList().SetChats(st.Chats())
	// Open a chat so the idle logo is hidden; otherwise its tick loop re-arms and
	// the no-op event would appear to return a command (issue #147).
	m.Chat().SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})

	newM, cmd := m.Update(store.Event{
		Kind:   store.EventMuteUpdate,
		ChatID: 1,
		Muted:  true, // same as stored
	})
	_ = newM.(ui.RootModel)

	assert.Nil(t, cmd)
	chat, ok := st.GetChat(1)
	require.True(t, ok)
	assert.True(t, chat.IsMuted)
}

func TestRoot_EventEditMessage_UpdatesStoredText(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	edited := time.Unix(int64(1700000000), 0)
	newM, _ := m.Update(store.Event{
		Kind:    store.EventEditMessage,
		Message: store.Message{ID: 10, ChatID: 1, Text: "edited", EditDate: &edited},
	})
	_ = newM.(ui.RootModel)

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Equal(t, "edited", msgs[0].Text)
	require.NotNil(t, msgs[0].EditDate)
}

func TestRoot_ReactionUpdate_BumpsIndicatorOnOtherChat(t *testing.T) {
	m, st := newRootWithTwoChats(t)
	// Neither chat is open, so a reaction on chat 2 should bump its indicator.
	newM, _ := m.Update(store.Event{
		Kind:            store.EventReactionsUpdate,
		ChatID:          2,
		MsgID:           500,
		ReactionsUnread: true,
	})
	root := newM.(ui.RootModel)

	c, ok := st.GetChat(2)
	require.True(t, ok)
	assert.Equal(t, 1, c.UnreadReactionsCount)

	// The chat list reflects the new count.
	var chat2 store.Chat
	for _, ch := range root.ChatList().Chats() {
		if ch.ID == 2 {
			chat2 = ch
		}
	}
	assert.Equal(t, 1, chat2.UnreadReactionsCount)
}

func TestRoot_ReactionUpdate_OnOpenChat_ReadsReactions(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock) // chat 1 open and focused
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}, UnreadReactionsCount: 1})

	newM, cmd := m.Update(store.Event{
		Kind:            store.EventReactionsUpdate,
		ChatID:          1,
		MsgID:           500,
		ReactionsUnread: true,
	})
	_ = newM.(ui.RootModel)
	require.NotNil(t, cmd)

	// Invoking the command sends readReactions to the server.
	done := cmd()
	_ = done
	assert.Equal(t, 1, mock.readReactionsCalls)
}

func TestRoot_OpenChat_ClearsUnreadReactionsOptimistically(t *testing.T) {
	mock := &mockTGClient{}
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}, UnreadReactionsCount: 3})
	m := ui.NewRootModel(mock, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	_ = newM.(ui.RootModel)

	c, ok := st.GetChat(1)
	require.True(t, ok)
	assert.Equal(t, 0, c.UnreadReactionsCount, "opening a chat optimistically clears its unread reactions")
}

func TestRoot_NewMention_BumpsIndicatorOnOtherChat(t *testing.T) {
	m, st := newRootWithTwoChats(t)
	// Chat 2 is not open; an incoming mention there bumps its indicator.
	newM, _ := m.Update(store.Event{
		Kind: store.EventNewMessage,
		Message: store.Message{
			ID: 500, ChatID: 2, Mentioned: true, IsOut: false,
		},
	})
	root := newM.(ui.RootModel)

	c, ok := st.GetChat(2)
	require.True(t, ok)
	assert.Equal(t, 1, c.UnreadMentionsCount)

	var chat2 store.Chat
	for _, ch := range root.ChatList().Chats() {
		if ch.ID == 2 {
			chat2 = ch
		}
	}
	assert.Equal(t, 1, chat2.UnreadMentionsCount)
}

func TestRoot_OpenChat_ClearsUnreadMentionsOptimistically(t *testing.T) {
	mock := &mockTGClient{}
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}, UnreadMentionsCount: 3})
	m := ui.NewRootModel(mock, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	_ = newM.(ui.RootModel)

	c, ok := st.GetChat(1)
	require.True(t, ok)
	assert.Equal(t, 0, c.UnreadMentionsCount, "opening a chat optimistically clears its unread mentions")
}

func TestRoot_PasteMsg_WhenComposerFocused_InsertsText(t *testing.T) {
	m, _ := newRootWithOpenChat(t, &mockTGClient{})
	// enter insert mode → focuses composer
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = newM.(ui.RootModel)
	require.True(t, m.Chat().ComposerFocused())

	newM, _ = m.Update(tea.PasteMsg{Content: "pasted text"})
	m = newM.(ui.RootModel)

	assert.Equal(t, "pasted text", m.Chat().ComposerValue())
}

func TestRoot_PasteMsg_WhenSearchOpen_UpdatesQuery(t *testing.T) {
	m, _ := newRootWithOpenChat(t, &mockTGClient{})
	// open search
	newM, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = newM.(ui.RootModel)
	require.True(t, m.SearchActive())

	newM, _ = m.Update(tea.PasteMsg{Content: "alice"})
	m = newM.(ui.RootModel)

	require.True(t, m.SearchActive())
	assert.Equal(t, "alice", m.Search().Query())
}

func TestRoot_Esc_NormalMode_ClosesChatReturnsToChatList(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// Open a chat — this sets focus to FocusChat.
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 1, Title: "Alice"}})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusChat, m.CurrentFocus())

	// Press Esc in normal mode.
	newM, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
}

func TestRoot_SetTmpDir(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m.SetTmpDir("/tmp/tele-test")
	assert.Equal(t, "/tmp/tele-test", m.TmpDir())
}

// TestRoot_NewMessageEvent_AlreadyReadElsewhere reproduces issue #88.
// gotd dispatches OtherUpdates (including updateReadHistoryInbox) before NewMessages
// in a getDifference response. A message whose ID is covered by the read pointer
// must not produce a false unread badge when it arrives after the read event.
func TestRoot_NewMessageEvent_AlreadyReadElsewhere(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{
		ID:             2,
		Title:          "Bob",
		ReadInboxMaxID: 100,
		UnreadCount:    0,
	})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	m = newM.(ui.RootModel)

	// EventReadInbox arrives first (gotd OtherUpdates before NewMessages)
	newM, _ = m.Update(store.Event{
		Kind:      store.EventReadInbox,
		ChatID:    2,
		ReadMaxID: 100,
	})
	m = newM.(ui.RootModel)

	// EventNewMessage for a message already covered by the read pointer
	newM, _ = m.Update(store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ID: 99, ChatID: 2, Text: "read elsewhere"},
	})
	root := newM.(ui.RootModel)

	var chat2 store.Chat
	for _, c := range root.ChatList().Chats() {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 0, chat2.UnreadCount, "message read elsewhere must not increment unread badge")
}

// TestRoot_StartupCatchup_ServerReadClearsStaleBadge reproduces issue #88.
// At startup the updates manager begins replaying getDifference catch-up events
// BEFORE GetDialogs finishes. A new-message event arrives while the store still
// holds the previous session's read pointer, so the badge increments. The read
// acknowledgement for that chat is dropped during getDifference. GetDialogs then
// writes the authoritative server state (read elsewhere → UnreadCount 0), which
// must win — the list badge must not stay stuck on the stale increment.
func TestRoot_StartupCatchup_ServerReadClearsStaleBadge(t *testing.T) {
	st := store.NewMemory()
	// Persisted from previous session: read up to 100, no unread.
	st.SetChat(store.Chat{ID: 2, Title: "Bob", ReadInboxMaxID: 100, UnreadCount: 0})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// Catch-up: new message (already read on another client) arrives before
	// GetDialogs completes and before the read ack (which is dropped).
	newM, _ := m.Update(store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ID: 150, ChatID: 2, Text: "read elsewhere"},
	})
	m = newM.(ui.RootModel)

	// GetDialogs completes: server reports the chat as already read.
	st.SetChat(store.Chat{ID: 2, Title: "Bob", ReadInboxMaxID: 150, UnreadCount: 0})

	// Transition rebuilds the chat list from the authoritative store.
	newM, _ = m.Update(screens.TransitionToMainMsg{})
	root := newM.(ui.RootModel)

	var chat2 store.Chat
	for _, c := range root.ChatList().Chats() {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 0, chat2.UnreadCount, "authoritative server read state must clear stale unread badge")
}

func TestRoot_Space_OpensChatMenu_OnChatList(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "A", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(&mockTGClient{}, st, 50, false).
		WithScreen(ui.ScreenMain).WithFocus(ui.FocusChatList)
	// TransitionToMainMsg populates the chat list from the store.
	nm, _ := m.Update(screens.TransitionToMainMsg{})
	m = nm.(ui.RootModel)

	nm, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = nm.(ui.RootModel)
	assert.True(t, m.ChatMenuOpen())
}

// TestRoot_RebindChatListConfirmToL_OpensChat reproduces issue #132. "l" is a
// global focus-cycle key, but a chatlist-context override must win over global:
// pressing it opens the chat under the cursor instead of cycling focus.
func TestRoot_RebindChatListConfirmToL_OpensChat(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(&mockTGClient{}, st, 50, false).
		WithScreen(ui.ScreenMain).WithFocus(ui.FocusChatList)
	nm, _ := m.Update(screens.TransitionToMainMsg{})
	m = nm.(ui.RootModel)

	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"chatlist": {"confirm": {"l"}},
	})
	require.Empty(t, warns)
	m = m.WithKeyMap(km)

	nm, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = nm.(ui.RootModel)

	require.NotNil(t, cmd, "l must trigger the chatlist confirm action")
	_, ok := cmd().(screens.OpenChatMsg)
	assert.True(t, ok, "l must open the chat under cursor, not cycle focus")
	assert.Equal(t, ui.FocusChatList, m.CurrentFocus(), "focus must not cycle to the chat pane")
}

func TestRoot_ToggleMute_OptimisticUpdate(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "A", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(&mockTGClient{}, st, 50, false).WithScreen(ui.ScreenMain)

	updated, _ := m.Update(components.ToggleMuteRequest{Peer: store.Peer{ID: 1}, Muted: true})
	rm := updated.(ui.RootModel)
	assert.False(t, rm.ChatMenuOpen(), "menu closes after action")

	c, _ := st.GetChat(1)
	assert.True(t, c.IsMuted, "store updated optimistically")
}

func TestRoot_MarkUnread_OptimisticUpdate(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(&mockTGClient{}, st, 50, false).WithScreen(ui.ScreenMain)

	m.Update(components.ToggleUnreadRequest{Peer: store.Peer{ID: 1}, Unread: true})
	c, _ := st.GetChat(1)
	assert.True(t, c.UnreadMark)
}

func TestRoot_ToggleArchive_OptimisticUpdate(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(&mockTGClient{}, st, 50, false).WithScreen(ui.ScreenMain)

	m.Update(components.ToggleArchiveRequest{Peer: store.Peer{ID: 1}, Archived: true})
	c, _ := st.GetChat(1)
	assert.True(t, c.IsArchived)
}

func TestRoot_EventEditMessage_HiddenEdit_DoesNotMarkEdited(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// A hidden edit (edit_hide) reaches the root as EventEditMessage with a nil
	// EditDate — e.g. a reaction bump. It must not flip the message to "edited"
	// (issue #118).
	newM, _ := m.Update(store.Event{
		Kind:    store.EventEditMessage,
		Message: store.Message{ID: 10, ChatID: 1, Text: "original", EditDate: nil},
	})
	_ = newM.(ui.RootModel)

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Nil(t, msgs[0].EditDate, "hidden edit must not set the edited marker")
}

func TestRoot_EventEditMessage_HiddenEdit_AppliesReactions(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// In a 1:1 chat an incoming reaction is delivered as a hidden edit
	// (edit_hide) carrying the message's new reactions, not as a separate
	// UpdateMessageReactions. The reactions must be applied so they appear live,
	// while the message must still not be flipped to "edited" (#160, #118).
	newM, _ := m.Update(store.Event{
		Kind: store.EventEditMessage,
		Message: store.Message{
			ID: 10, ChatID: 1, Text: "original", EditDate: nil,
			Reactions: []store.Reaction{{Emoji: "👍", Count: 1}},
		},
	})
	_ = newM.(ui.RootModel)

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Nil(t, msgs[0].EditDate, "hidden edit must not set the edited marker")
	require.Len(t, msgs[0].Reactions, 1, "reactions from the hidden edit must be applied")
	assert.Equal(t, "👍", msgs[0].Reactions[0].Emoji)
	assert.Equal(t, 1, msgs[0].Reactions[0].Count)
}

func TestRoot_SearchUsersRequestRunsRPCAndRoutesResult(t *testing.T) {
	mock := &mockTGClient{searchResult: []store.Chat{
		{ID: 99, Title: "Zoe", Peer: store.Peer{ID: 99, Type: store.PeerUser}},
	}}
	st := store.NewMemory()
	m := ui.NewRootModel(mock, st, 20, false).WithScreen(ui.ScreenMain)

	_, cmd := m.Update(screens.SearchUsersRequest{Query: "zo", Serial: 1})
	require.NotNil(t, cmd, "SearchUsersRequest should produce a command")
	var res screens.SearchUsersResult
	var found bool
	for _, mm := range drainMsgs(cmd()) {
		if r, ok := mm.(screens.SearchUsersResult); ok {
			res, found = r, true
		}
	}
	require.True(t, found, "expected a SearchUsersResult")
	assert.Equal(t, "zo", mock.lastSearchQuery)
	require.Len(t, res.Chats, 1)
	assert.Equal(t, int64(99), res.Chats[0].ID)
	assert.Equal(t, 1, res.Serial)
}
