package ui_test

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setChatListWindow loads a window holding exactly these chats, converting them
// to the projection rows the component consumes — the job the core does in
// production. Fixtures go on describing chats.
func setChatListWindow(m *screens.ChatListModel, chats []domain.Chat) {
	rows := make([]project.ChatRow, 0, len(chats))
	for _, c := range chats {
		rows = append(rows, project.ChatRow{
			ID:         c.ID,
			Title:      c.Title,
			IsUser:     c.Peer.IsUser(),
			Online:     c.Online,
			Unread:     c.UnreadCount,
			Mentions:   c.UnreadMentionsCount,
			Reactions:  c.UnreadReactionsCount,
			UnreadMark: c.UnreadMark,
			Muted:      c.IsMuted,
		})
	}
	m.SetWindow(0, len(rows), rows)
}

// There is no Telegram client double here any more. The TUI holds an owner and
// nothing else since #195/#198, so what these tests stand in for is the owner:
// see testOwner in applyevent_export_test.go.

// A refused submission is the only synchronous failure a send has left: the
// owner could not address the chat. Everything past that is the queue's, and
// there is no optimistic message to roll back (#193).
func TestRoot_SubmissionRefused_SurfacesError(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	owner := m.Owner().(*testOwner)
	owner.cmdErr = &telerr.Error{Kind: telerr.PeerNotFound}

	_, cmd := m.Update(screens.SendMsgRequest{Text: "hi"})
	require.NotNil(t, cmd)

	_, isErr := cmd().(ui.StatusErrMsg)
	assert.True(t, isErr, "a refused submission should surface a StatusErrMsg")
	assert.Empty(t, st.Messages(1), "nothing is written to the store on the send path any more")
	assert.Empty(t, owner.sent)
}

type ctxKey struct{}

func TestRoot_ThreadsAppContextIntoCommands(t *testing.T) {
	appCtx := context.WithValue(context.Background(), ctxKey{}, "tele")
	m, _ := newRootWithOpenChat(t)
	m = m.WithContext(appCtx)
	owner := m.Owner().(*testOwner)

	_, cmd := m.Update(screens.SendMsgRequest{Text: "hi"})
	require.NotNil(t, cmd)
	cmd() // run the submission cmd

	require.NotNil(t, owner.lastSendCtx)
	assert.Equal(t, "tele", owner.lastSendCtx.Value(ctxKey{}),
		"command should use the app context, not context.Background()")
}

func TestRoot_ReactionFailure_SurfacesError(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "hi", Date: time.Now()})
	applyHistory(t, m, st, 1)

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
	m, st := newRootWithOpenChat(t) // chat ID 1 is the active chat

	newMsg := domain.Message{ID: 101, ChatID: 1, Photo: &domain.PhotoRef{ID: 9}}
	_, cmd := applyEvent(t, m, st, store.Event{Kind: store.EventNewMessage, Message: newMsg})
	require.NotNil(t, cmd) // download command batched
}

func TestRoot_Draft_FlushedToServerOnChatSwitch(t *testing.T) {
	m, st := newRootWithOpenChat(t) // chat 1 open
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", Peer: domain.Peer{ID: 2, Type: domain.PeerUser}})

	// Type a draft into chat 1's composer, then switch to chat 2.
	m = m.SetComposerValueForTest("hello Alice")
	_, cmd := m.Update(screens.OpenChatMsg{ChatID: 2, Title: "Bob"})
	require.NotNil(t, cmd)
	drainMsgs(cmd()) // executes the batched SaveDraft side effect

	o := ownerOf(t, m)
	var found bool
	for _, d := range o.savedDrafts {
		if d.chatID == 1 && d.text == "hello Alice" {
			found = true
		}
	}
	assert.True(t, found, "switching chats must persist the outgoing draft; got %+v", o.savedDrafts)
}

func TestRoot_Draft_EmptyComposerNoServerSave(t *testing.T) {
	m, st := newRootWithOpenChat(t) // chat 1 open, composer empty
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", Peer: domain.Peer{ID: 2, Type: domain.PeerUser}})

	// Switch away without typing — no draft change, so no network save.
	_, cmd := m.Update(screens.OpenChatMsg{ChatID: 2, Title: "Bob"})
	if cmd != nil {
		drainMsgs(cmd())
	}
	assert.Empty(t, ownerOf(t, m).savedDrafts, "no draft change must not hit messages.saveDraft")
}

func TestRoot_EventDraftMessage_UpdatesStore(t *testing.T) {
	m, st := newRootWithOpenChat(t)

	applyEvent(t, m, st, store.Event{Kind: store.EventDraftMessage, ChatID: 1, Draft: "remote draft"})
	got, ok := st.GetChat(1)
	require.True(t, ok)
	assert.Equal(t, "remote draft", got.Draft)
}

func TestPendingDownloadCmds_GIFThumb_FiresDownload(t *testing.T) {
	m, _ := newRootWithOpenChat(t) // chat ID 1 is the active chat

	gif := domain.Message{
		ID: 201, ChatID: 1,
		Media:    &domain.MediaRef{Kind: domain.MediaGIF},
		Document: &domain.DocumentRef{ID: 77, ThumbSize: "m"},
	}
	require.NotNil(t, m.PendingDownloadCmdsForTest([]domain.Message{gif}),
		"GIF with a thumb must fire a download")

	noThumb := domain.Message{
		ID: 202, ChatID: 1,
		Media:    &domain.MediaRef{Kind: domain.MediaGIF},
		Document: &domain.DocumentRef{ID: 78}, // no ThumbSize
	}
	assert.Nil(t, m.PendingDownloadCmdsForTest([]domain.Message{noThumb}),
		"GIF without a thumb must not fire a download")
}

func TestSaveGifFileCmd_EmitsPathOnSuccess(t *testing.T) {
	o := newTestOwner(store.NewMemory())
	src := filepath.Join(t.TempDir(), "anim.mp4")
	require.NoError(t, os.WriteFile(src, []byte("mp4"), 0600))
	o.mediaPaths[mediaPathKey{1, 10, domain.DocFull}] = src

	docID, path, ok := ui.GifFileReadyForTest(o, 1, 10, 77, t.TempDir())

	require.True(t, ok, "command must produce a gif-file-ready result")
	assert.Equal(t, int64(77), docID)
	assert.NotEmpty(t, path, "saved path must be set")
}

// Older history entering the window must schedule the downloads for the photos
// it brought, the same as the initial window did.
func TestRoot_OlderHistory_FiresPhotoDownload(t *testing.T) {
	m, st := newRootWithOpenChat(t) // chat ID 1 is the active chat

	st.SetMessages(1, []domain.Message{
		{ID: 150, ChatID: 1, Photo: &domain.PhotoRef{ID: 5}, Date: time.Unix(1, 0)},
	})
	_, cmd := applyHistory(t, m, st, 1)
	require.NotNil(t, cmd)
}

func TestRoot_ChatOpenFailure_ClearsSpinnerAndShowsError(t *testing.T) {
	st := store.NewMemory()
	chat := domain.Chat{ID: 7, Title: "Bob", Peer: domain.Peer{ID: 7, Type: domain.PeerUser}}
	st.SetChat(chat)
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(ui.RootModel)

	m = openChat(t, m, chat.ID, chat.Title)

	// The client cannot see whether filling the window touched the network, so
	// the owner has to say when it could not: otherwise the pane waits forever.
	newM, _ = m.Update(core.Failure{ChatID: chat.ID, Op: "load history", Err: errors.New("timeout")})
	m = newM.(ui.RootModel)

	assert.Contains(t, m.View().Content, "timeout")
}

// Inline fetching moved behind the owner in #196. The refresh-on-expired-ref
// retry is covered by TestFetchMedia_RefreshesAnExpiredReferenceAndRecordsIt in
// internal/core, and the sticker path by TestFetchStickerCmd_DecodesAWebpFile.

// drainMsgs flattens a (possibly batched) cmd result into its concrete messages.
// newRootOnChat builds a main-screen RootModel focused on a single chat (id 1),
// draining the open-chat command so history loading settles.
func newRootOnChat(t *testing.T) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	chat := domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}}
	st.SetChat(chat)
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(ui.RootModel)
	return openChat(t, m, chat.ID, chat.Title), st
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
	m, _ := newRootOnChat(t)

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
	m, _ := newRootOnChat(t)
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
	m, _ := newRootOnChat(t)
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
	m, _ := newRootOnChat(t)
	nm, _ := m.Update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	m = nm.(ui.RootModel)
	if !m.FilePickerOpen() {
		t.Fatal("file picker not open after 'u'")
	}
	if !strings.Contains(m.View().Content, "filter") {
		t.Fatalf("open file picker is not rendered in the view:\n%s", m.View().Content)
	}
}

// What the client is responsible for is handing the staged files over. The
// upload, the album assembly, the InputMedia and what a failure means are all on
// the owner's side of this call since #195 — see internal/core.
func TestSendMedia_HandsTheStagedFilesToTheQueue(t *testing.T) {
	m, st := newRootOnChat(t)
	o := m.Owner().(*testOwner)

	first := writeTempFile(t, "a.jpg", "hello")
	second := writeTempFile(t, "b.jpg", "there")
	for _, p := range []string{first, second} {
		nm, _ := m.Update(screens.FileSelectedMsg{Path: p})
		m = nm.(ui.RootModel)
	}

	nm, cmd := m.Update(screens.SendMediaRequest{
		Caption: "hi", ReplyToMsgID: 7,
	})
	m = nm.(ui.RootModel)
	require.NotNil(t, cmd)
	cmd()

	require.Len(t, o.sentMedia, 1, "one submission is one request, however many files it holds")
	req := o.sentMedia[0]
	assert.NotEmpty(t, req.Ref, "the client owns the idempotency key")
	assert.Equal(t, int64(1), req.ChatID)
	assert.Equal(t, []string{first, second}, mediaPathsOf(req.Files))
	assert.Equal(t, "hi", req.Caption)
	assert.Equal(t, 7, req.ReplyToMsgID)

	assert.Empty(t, st.Messages(1),
		"nothing is written locally: the queue's own entry is what appears on screen")
	assert.Zero(t, m.PendingAttachmentCount(), "the staged queue is handed over, not kept")
}

// The "send as file" choice is the one piece of intent the owner cannot work out
// for itself, so it travels with the request. Everything else it detects.
func TestSendMedia_CarriesTheSendAsChoice(t *testing.T) {
	m, _ := newRootOnChat(t)
	o := m.Owner().(*testOwner)

	nm, _ := m.Update(screens.FileSelectedMsg{Path: writeTempBytes(t, "a.png", pngBytes)})
	m = nm.(ui.RootModel)
	m = pressToggleSendAs(t, m)

	_, cmd := m.Update(screens.SendMediaRequest{})
	require.NotNil(t, cmd)
	cmd()

	require.Len(t, o.sentMedia, 1)
	require.Len(t, o.sentMedia[0].Files, 1)
	assert.Equal(t, domain.MediaFile, o.sentMedia[0].Files[0].SendAs)
}

func mediaPathsOf(files []core.MediaFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

// Which InputMedia a staged file becomes — a forced document, a streamable
// video, a photo — is decided in internal/core now. See TestUploadPart_* there.

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
	m := ui.NewRootModel(nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	newM, cmd := newM.(ui.RootModel).Update(ui.StatusErrMsg{Text: "network down", Sev: components.SeverityError})
	root := newM.(ui.RootModel)
	root.SettleToastsForTest()
	assert.Contains(t, root.View().Content, "network down")
	require.NotNil(t, cmd) // an auto-clear tick was scheduled
}

func TestRoot_ClearStatusErrMsg_StaleSerialKeepsError(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false).WithScreen(ui.ScreenMain)
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
	m := ui.NewRootModel(nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	done := ui.DocumentOpenDoneMsgForTest(1, "open file failed: boom", components.SeverityWarning)
	m2, _ := newM.(ui.RootModel).Update(done)
	root := m2.(ui.RootModel)
	root.SettleToastsForTest()
	assert.Contains(t, root.View().Content, "open file failed: boom")
}

// A successful completion adds no error text to the status bar.
func TestRoot_DocumentOpenDone_SuccessNoError(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	done := ui.DocumentOpenDoneMsgForTest(1, "", components.SeverityWarning)
	m2, _ := newM.(ui.RootModel).Update(done)
	assert.NotContains(t, m2.(ui.RootModel).View().Content, "failed")
}

func TestRoot_FileDownloadDone_SuccessShowsPath(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	done := ui.FileDownloadDoneMsgForTest(1, "Saved to /tmp/report.pdf", components.SeverityInfo)
	m2, _ := newM.(ui.RootModel).Update(done)
	root := m2.(ui.RootModel)
	root.SettleToastsForTest()
	assert.Contains(t, root.View().Content, "Saved to /tmp/report.pdf")
}

func TestRoot_FileDownloadDone_ErrorShowsText(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false).WithScreen(ui.ScreenMain)
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
	fileMsg := domain.Message{
		ID: 7, ChatID: 1, Date: time.Now(),
		Media:    &domain.MediaRef{Kind: domain.MediaFile},
		Document: &domain.DocumentRef{ID: 5, FileName: "report.pdf"},
	}
	m, st := newRootOnChat(t)
	st.SetMessages(1, []domain.Message{fileMsg})
	nm, _ := applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)
	_ = m.View() // establish the selected message

	m2, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	// The status-bar download indicator becomes active. It names the media kind
	// rather than the file: the owner picks the name now, so the client has none
	// to show when the download starts (#196).
	assert.Contains(t, m2.(ui.RootModel).View().Content, "downloading file…")
}

func TestRoot_InitialScreen_Login(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	assert.Equal(t, ui.ScreenLogin, m.CurrentScreen())
}

func TestRoot_InitialChatList_IsFocused(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	assert.True(t, m.ChatList().Focused(), "chatList must be focused from the start so cursor highlight is visible")
}

func TestRoot_2_FocusesChat(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
	newM, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChat, root.CurrentFocus())
}

func TestRoot_1_FocusesChatList(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m = m.WithFocus(ui.FocusChat)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChatList, root.CurrentFocus())
}

func TestRoot_TransitionToMain(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.ScreenMain, root.CurrentScreen())
}

func TestRoot_CtrlC_Quits(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	// Settle the animation loops so the returned command is the quit alone, not
	// batched with an animation re-arm (issue #147).
	setChatListWindow(m.ChatList(), []domain.Chat{{ID: 1}})
	m.Chat().SetMessages([]domain.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	assert.NotNil(t, cmd)
	msg := cmd()
	_, isQuit := msg.(tea.QuitMsg)
	assert.True(t, isQuit)
}

// Reaching the top of the window widens it. The client asks for more history; it
// no longer fetches, and no longer knows whether the extra messages come from
// the store or from Telegram.
func TestRoot_LoadMore_WidensTheChatWindow(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)
	m = openChat(t, m, 1, "Alice")
	o := m.Owner().(*testOwner)

	m.Update(screens.LoadMoreMsg{ChatID: 1, OffsetID: 5})

	w, ok := o.lastChatWindow()
	require.True(t, ok, "reaching the top must move the chat window")
	assert.Equal(t, int64(1), w.ChatID)
	assert.Greater(t, w.Before, 50, "the window must ask for more than it already had")
}

// Repeating it keeps widening; collapsing duplicate fetches is the owner's job
// (see TestOwner_ConcurrentBackfillsCollapseToOneFetch), not the client's.
func TestRoot_LoadMore_WidensAgainOnRepeat(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)
	m = openChat(t, m, 1, "Alice")
	o := m.Owner().(*testOwner)

	next, _ := m.Update(screens.LoadMoreMsg{ChatID: 1, OffsetID: 5})
	first, _ := o.lastChatWindow()
	next.(ui.RootModel).Update(screens.LoadMoreMsg{ChatID: 1, OffsetID: 5})

	second, ok := o.lastChatWindow()
	require.True(t, ok)
	assert.Greater(t, second.Before, first.Before)
}

// A load-more for a chat that is not open is ignored: it belongs to a window
// nobody is looking at.
func TestRoot_LoadMore_IgnoredForAnotherChat(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)
	m = openChat(t, m, 1, "Alice")
	o := m.Owner().(*testOwner)
	o.moves = nil

	m.Update(screens.LoadMoreMsg{ChatID: 99, OffsetID: 5})

	_, ok := o.lastChatWindow()
	assert.False(t, ok)
}

func TestRoot_SlashKey_ActivatesSearch(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice"})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	root := newM.(ui.RootModel)
	assert.True(t, root.SearchActive())
}

func TestRoot_CloseSearchMsg_DeactivatesSearch(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice"})
	m := newRoot(st, 50, false)
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
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1}})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = newM.(ui.RootModel)
	newM, _ = m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Alice"})
	m = newM.(ui.RootModel)
	assert.False(t, m.SearchActive())
}

func newRootWithTwoChats(t *testing.T) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice"})
	st.SetChat(domain.Chat{ID: 2, Title: "Bob"})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	return newM.(ui.RootModel), st
}

func TestRoot_NewMessageEvent_UpdatesChatList(t *testing.T) {
	m, st := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: domain.Message{ChatID: 2, Text: "hi", Date: time.Now()},
	}
	newM, _ := applyEvent(t, m, st, evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Rows()
	require.Len(t, chats, 2)
	assert.Equal(t, int64(2), chats[0].ID, "chat 2 should bubble to top after new message")
}

func TestRoot_NewMessageEvent_IncrementsUnread(t *testing.T) {
	m, st := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: domain.Message{ID: 1, ChatID: 2, Text: "hi"},
	}
	newM, _ := applyEvent(t, m, st, evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Rows()
	var chat2 project.ChatRow
	for _, c := range chats {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 1, chat2.Unread)
}

func TestRoot_NewMessageEvent_UnreadPersistsAcrossMultipleEvents(t *testing.T) {
	m, st := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: domain.Message{ID: 1, ChatID: 2, Text: "first"},
	}
	newM, _ := applyEvent(t, m, st, evt)
	m = newM.(ui.RootModel)

	evt2 := store.Event{
		Kind:    store.EventNewMessage,
		Message: domain.Message{ID: 2, ChatID: 2, Text: "second"},
	}
	newM, _ = applyEvent(t, m, st, evt2)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Rows()
	var chat2 project.ChatRow
	for _, c := range chats {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 2, chat2.Unread, "unread count should accumulate across multiple new-message events")
}

// Unread is account state, not a property of the open viewport: a message in the
// open chat counts, and the MarkRead round trip clears it (#189).
func TestRoot_NewMessageEvent_CountsUnreadForCurrentChat(t *testing.T) {
	m, st := newRootWithTwoChats(t)

	newM, _ := m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Alice"})
	m = newM.(ui.RootModel)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: domain.Message{ID: 7, ChatID: 1, Text: "hi"},
	}
	newM, _ = applyEvent(t, m, st, evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Rows()
	var chat1 project.ChatRow
	for _, c := range chats {
		if c.ID == 1 {
			chat1 = c
		}
	}
	assert.Equal(t, 1, chat1.Unread)
}

func TestRoot_NewMessageEvent_NoUnreadForOutgoingMessage(t *testing.T) {
	m, st := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: domain.Message{ChatID: 2, Text: "sent from phone", IsOut: true},
	}
	newM, _ := applyEvent(t, m, st, evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Rows()
	var chat2 project.ChatRow
	for _, c := range chats {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 0, chat2.Unread)
}

func newRootWithOpenChat(t *testing.T) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	return openChat(t, m, 1, "Alice"), st
}

// Sending submits to the durable queue and writes nothing locally. What appears
// on screen is the queue's own entry, arriving through the projection (#193).
func TestRoot_Send_SubmitsToTheQueueAndTouchesNoStore(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	owner := m.Owner().(*testOwner)

	_, cmd := m.Update(screens.SendMsgRequest{
		Text: "hello",
	})
	require.NotNil(t, cmd)
	cmd()

	require.Len(t, owner.sent, 1)
	assert.Equal(t, "hello", owner.sent[0].Text)
	assert.Equal(t, int64(1), owner.sent[0].ChatID)
	assert.NotEmpty(t, owner.sent[0].Ref, "the client owns the idempotency key")
	assert.Empty(t, st.Messages(1), "no optimistic message is inserted any more")
}

func TestRootModel_PhotoDownloadDispatchedOnHistory(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.SetMessages(1, []domain.Message{
		{ID: 10, ChatID: 1, Text: "hello", Date: time.Unix(1, 0)},
		{ID: 11, ChatID: 1, Photo: &domain.PhotoRef{ID: 77, ThumbSize: "m"}, Date: time.Unix(2, 0)},
	})
	_, cmd := applyHistory(t, m, st, 1)
	require.NotNil(t, cmd, "should return cmd (download + markread) for messages with photo")
}

func TestRootModel_PhotoReadyMsg_StoresImage(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	m2, _ := m.Update(ui.PhotoReadyMsg{PhotoID: 55, Image: img})
	_ = m2
	// No panic — image cache updated without crashing
}

// Two sends must carry distinct refs: the ref is the idempotency key, and a
// repeated one would make the second message a no-op resubmission of the first.
func TestRoot_Send_ConcurrentSubmissionsHaveDistinctRefs(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	owner := m.Owner().(*testOwner)

	newM, first := m.Update(screens.SendMsgRequest{
		Text: "first",
	})
	m = newM.(ui.RootModel)
	require.NotNil(t, first)
	first()

	_, second := m.Update(screens.SendMsgRequest{
		Text: "second",
	})
	require.NotNil(t, second)
	second()

	require.Len(t, owner.sent, 2)
	assert.NotEqual(t, owner.sent[0].Ref, owner.sent[1].Ref)
}

func TestRoot_Space_OpensContextMenu(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)

	assert.True(t, m.ContextMenuOpen())
}

func TestRoot_Space_NoMenuWhenNoMessages(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	// No messages added — SelectedMessageID() returns 0

	newM, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)

	assert.False(t, m.ContextMenuOpen(), "menu should not open when no message is selected")
}

func TestRoot_EscKeepsReply_XClearsIt(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "hi", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
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
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 11, ChatID: 1, Text: "mine", IsOut: true, Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
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
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
	m = newM.(ui.RootModel)
	require.False(t, m.ReactionPickerOpen())

	newM, _ = m.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = newM.(ui.RootModel)
	assert.True(t, m.ReactionPickerOpen(), "react key should open the reaction picker")
}

func TestRoot_ForwardKey_OpensPicker(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
	m = newM.(ui.RootModel)
	require.False(t, m.SearchActive())

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = newM.(ui.RootModel)
	assert.True(t, m.SearchActive(), "forward key should open the chat picker")
}

func TestRoot_ForwardToChat_AsksTheOwner(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	target := int64(999)

	newM, cmd := m.Update(screens.ForwardToChatRequest{ToChatID: target, MsgID: 5})
	m = newM.(ui.RootModel)
	require.False(t, m.SearchActive(), "picker should close on confirm")
	require.NotNil(t, cmd)
	drainMsgs(cmd())

	o := ownerOf(t, m)
	assert.Equal(t, target, o.forwardTo)
	assert.Equal(t, []int{5}, o.forwardIDs)
	assert.Equal(t, int64(1), o.forwardFrom, "source is the open chat")
}

// The comment travels with the command; sending it ahead of the forward is the
// owner's business now (#198), and becomes an outbox submission in #193.
func TestRoot_ForwardWithComment_PassesTheComment(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	target := int64(999)

	_, cmd := m.Update(screens.ForwardToChatRequest{ToChatID: target, MsgID: 5, Comment: "look at this"})
	require.NotNil(t, cmd)
	drainMsgs(cmd())

	o := ownerOf(t, m)
	assert.Equal(t, "look at this", o.forwardComment)
	assert.Equal(t, target, o.forwardTo)
	assert.Equal(t, []int{5}, o.forwardIDs)
}

func TestRoot_ForwardWithoutComment_PassesNoComment(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	target := int64(999)

	_, cmd := m.Update(screens.ForwardToChatRequest{ToChatID: target, MsgID: 5})
	require.NotNil(t, cmd)
	drainMsgs(cmd())

	o := ownerOf(t, m)
	assert.Empty(t, o.forwardComment, "no comment means nothing extra to send")
	assert.Equal(t, []int{5}, o.forwardIDs)
}

func TestRoot_Forward_BumpsTargetChatToTop(t *testing.T) {
	m, st := newRootWithOpenChat(t) // chat 1 is open/current
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", Peer: domain.Peer{ID: 2, Type: domain.PeerUser}})
	// Source message lives in the open chat; chat 1 has an older last message.
	st.AppendMessage(domain.Message{ID: 7, ChatID: 1, Text: "src", Date: time.Now().Add(-time.Hour)})

	target := int64(2)
	_, cmd := m.Update(screens.ForwardToChatRequest{ToChatID: target, MsgID: 7})
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
	m, _ := newRootWithOpenChat(t)
	// The refusal comes back from the owner's command now.
	ownerOf(t, m).cmdErr = &telerr.Error{Kind: telerr.Forbidden, Detail: "CHAT_FORWARDS_RESTRICTED"}
	target := int64(999)

	_, cmd := m.Update(screens.ForwardToChatRequest{ToChatID: target, MsgID: 5})
	require.NotNil(t, cmd)
	done := cmd()
	_, cmd2 := m.Update(done)
	require.NotNil(t, cmd2)
	se, ok := cmd2().(ui.StatusErrMsg)
	require.True(t, ok, "restricted forward should surface a StatusErrMsg")
	assert.Equal(t, "forward: not allowed in this chat", se.Text)
	assert.Equal(t, components.SeverityWarning, se.Sev)
}

func TestRoot_ContextMenu_EscCloses(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
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
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
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
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
	m = newM.(ui.RootModel)

	require.Len(t, st.Messages(1), 1)

	// Deleting is the owner's command now, so the removal lands when it runs.
	newM, cmd := m.Update(components.DeleteMsgRequest{MsgID: 10, Revoke: false})
	_ = newM
	require.NotNil(t, cmd, "the request must produce an owner command")
	drainMsgs(cmd())

	assert.Empty(t, st.Messages(1), "message removed from store")
}

func TestRoot_ContextMenu_QuitKeyDoesNotQuit(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
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
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "original", SenderName: "Alice", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
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
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "original", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
	m = newM.(ui.RootModel)

	owner := m.Owner().(*testOwner)

	_, cmd := m.Update(screens.SendMsgRequest{
		Text:         "my reply",
		ReplyToMsgID: 10,
	})
	require.NotNil(t, cmd)
	cmd() // submits to the queue

	require.Len(t, owner.sent, 1)
	assert.Equal(t, 10, owner.sent[0].ReplyToMsgID)
}

func TestRoot_R_Key_ActivatesReplyMode(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "original", SenderName: "Alice", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = newM.(ui.RootModel)

	assert.True(t, m.Chat().ComposerFocused(), "r key must activate reply mode and focus composer")
	assert.Equal(t, 10, m.Chat().ReplyToMsgID(), "r key must set reply target")
	assert.Equal(t, keys.ModeInsert, m.VimMode(), "r key must switch root to insert mode")
}

func TestRoot_OpenChat_ClearsPendingReply(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "original", SenderName: "Alice", Date: time.Now()})
	newM, _ := applyHistory(t, m, st, 1)
	m = newM.(ui.RootModel)

	// activate reply mode
	newM, _ = m.Update(components.ReplyMsgRequest{MsgID: 10})
	m = newM.(ui.RootModel)
	require.Equal(t, 10, m.Chat().ReplyToMsgID(), "reply must be active before switching chat")

	// switch to a different chat
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", Peer: domain.Peer{ID: 2, Type: domain.PeerUser}})
	newM, _ = m.Update(screens.OpenChatMsg{ChatID: 2, Title: "Bob"})
	m = newM.(ui.RootModel)

	assert.Equal(t, 0, m.Chat().ReplyToMsgID(), "switching chat must clear pending reply")
}

func TestRoot_h_CyclesFocusLeft(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m = m.WithFocus(ui.FocusChat)
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChatList, root.CurrentFocus())
}

func TestRoot_l_CyclesFocusRight(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChat, root.CurrentFocus())
}

func TestRoot_FolderSelectedMsg_FiltersChatList(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, IsContact: true})
	st.SetChat(domain.Chat{ID: 2, Title: "Group", Peer: domain.Peer{ID: 2, Type: domain.PeerGroup}})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	m = toMain(t, m)

	// The owner stores the filters and then tells the client about them; the
	// core resolves a folder id against the stored set when it windows the list.
	filter := domain.FolderFilter{ID: 1, Title: "Contacts", Contacts: true}
	st.SetFolderFilters([]domain.FolderFilter{filter})
	newM, _ := m.Update(ui.FolderFiltersMsg{Filters: []domain.FolderFilter{filter}})
	m = newM.(ui.RootModel)

	// Selecting a folder repoints the window; the core does the filtering.
	selectedFilter := filter
	newM, _ = m.Update(screens.FolderSelectedMsg{Filter: &selectedFilter})
	root := drainOwner(t, newM.(ui.RootModel))

	w, ok := root.Owner().(*testOwner).lastChatListWindow()
	require.True(t, ok, "selecting a folder must move the chatlist window")
	assert.Equal(t, 1, w.Folder)

	chats := root.ChatList().Rows()
	require.Len(t, chats, 1)
	assert.Equal(t, int64(1), chats[0].ID)
}

func TestRoot_FolderFiltersMsg_SetsFolders(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	filters := []domain.FolderFilter{{ID: 1, Title: "Work"}}
	newM, _ := m.Update(ui.FolderFiltersMsg{Filters: filters})
	root := newM.(ui.RootModel)
	assert.True(t, root.HasFolders())
}

func TestRoot_0_FocusesFolders(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	filters := []domain.FolderFilter{{ID: 1, Title: "Work"}}
	m2, _ := m.Update(ui.FolderFiltersMsg{Filters: filters})
	root := m2.(ui.RootModel)
	newM, _ := root.Update(tea.KeyPressMsg{Code: '0', Text: "0"})
	root2 := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusFolders, root2.CurrentFocus())
}

func TestRoot_FocusNext_DoesNotAutoOpenChat(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice"})
	st.SetChat(domain.Chat{ID: 2, Title: "Bob"})
	m := newRoot(st, 50, false)
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
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	filters := []domain.FolderFilter{{ID: 1, Title: "Work"}}
	newM, _ := m.Update(ui.FolderFiltersMsg{Filters: filters})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: '0', Text: "0"})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusFolders, m.CurrentFocus())

	filter := domain.FolderFilter{ID: 1, Title: "Work"}
	newM, _ = m.Update(screens.FolderSelectedMsg{Filter: &filter})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
}

func TestRoot_OpenSameChatAgain_OnlyFocusesChatPane(t *testing.T) {
	m, _ := newRootWithOpenChat(t)

	newM, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusChatList, m.CurrentFocus())

	newM, cmd := m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Alice"})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChat, m.CurrentFocus())
	assert.Nil(t, cmd, "re-opening same chat must not trigger a history reload")
}

func TestRoot_EventDeleteMessages_Channel_RemovesFromCurrentChat(t *testing.T) {
	m, st := newRootWithTwoChats(t)
	now := time.Now()
	st.SetMessages(1, []domain.Message{
		{ID: 10, ChatID: 1, Text: "hello", Date: now},
		{ID: 11, ChatID: 1, Text: "world", Date: now},
	})
	newM, _ := m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Alice"})
	m = newM.(ui.RootModel)

	evt := store.Event{
		Kind:   store.EventDeleteMessages,
		ChatID: 1,
		MsgIDs: []int{10},
	}
	newM, _ = applyEvent(t, m, st, evt)
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
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Text: "a", Date: now}})
	st.SetMessages(2, []domain.Message{{ID: 6, ChatID: 2, Text: "b", Date: now}})

	evt := store.Event{
		Kind:   store.EventDeleteMessages,
		ChatID: 0,
		MsgIDs: []int{5},
	}
	newM, _ := applyEvent(t, m, st, evt)
	_ = newM.(ui.RootModel)

	assert.Empty(t, st.Messages(1))   // owning chat lost the message
	require.Len(t, st.Messages(2), 1) // unrelated chat untouched
}

func TestRoot_ContextMenu_PhotoMessage_ShowsAllThreeActions(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(ui.RootModel)
	st.AppendMessage(domain.Message{
		ID:     10,
		ChatID: 1,
		Text:   "photo msg",
		Photo:  &domain.PhotoRef{ID: 77},
		Date:   time.Now(),
	})
	newM, _ = applyHistory(t, m, st, 1)
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
	m, st := newRootWithOpenChat(t)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(ui.RootModel)
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "text msg", Date: time.Now()})
	newM, _ = applyHistory(t, m, st, 1)
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
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	setChatListWindow(m.ChatList(), st.Chats())

	newM, _ := applyEvent(t, m, st, store.Event{
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
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, Online: true})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	setChatListWindow(m.ChatList(), st.Chats())
	// Open a chat so the idle logo is hidden; otherwise its tick loop re-arms and
	// the no-op event would appear to return a command (issue #147).
	m.Chat().SetMessages([]domain.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})

	newM, cmd := applyEvent(t, m, st, store.Event{
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
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	setChatListWindow(m.ChatList(), st.Chats())

	newM, _ := applyEvent(t, m, st, store.Event{
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
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, IsMuted: true})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	setChatListWindow(m.ChatList(), st.Chats())
	// Open a chat so the idle logo is hidden; otherwise its tick loop re-arms and
	// the no-op event would appear to return a command (issue #147).
	m.Chat().SetMessages([]domain.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})

	newM, cmd := applyEvent(t, m, st, store.Event{
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
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "original"})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	edited := time.Unix(int64(1700000000), 0)
	newM, _ := applyEvent(t, m, st, store.Event{
		Kind:    store.EventEditMessage,
		Message: domain.Message{ID: 10, ChatID: 1, Text: "edited", EditDate: &edited},
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
	newM, _ := applyEvent(t, m, st, store.Event{
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
	var chat2 project.ChatRow
	for _, ch := range root.ChatList().Rows() {
		if ch.ID == 2 {
			chat2 = ch
		}
	}
	assert.Equal(t, 1, chat2.Reactions)
}

// A reaction on a message that was edited earlier arrives as an edit update
// carrying a non-nil EditDate. The chat list must still pick up the indicator,
// exactly as it does for a plain reactions update (#199).
func TestRoot_EditWithReaction_BumpsIndicatorOnOtherChat(t *testing.T) {
	m, st := newRootWithTwoChats(t)
	edited := time.Now().Add(-time.Hour)
	st.AppendMessage(domain.Message{ID: 500, ChatID: 2, Text: "fixed typo", EditDate: &edited})

	newM, _ := applyEvent(t, m, st, store.Event{
		Kind: store.EventEditMessage,
		Message: domain.Message{
			ID: 500, ChatID: 2, Text: "fixed typo", EditDate: &edited,
			Reactions:          []domain.Reaction{{Emoji: "👍", Count: 1}},
			HasUnreadReactions: true,
		},
	})
	root := newM.(ui.RootModel)

	var chat2 project.ChatRow
	for _, ch := range root.ChatList().Rows() {
		if ch.ID == 2 {
			chat2 = ch
		}
	}
	assert.Equal(t, 1, chat2.Reactions)
}

func TestRoot_ReactionUpdate_OnOpenChat_ReadsReactions(t *testing.T) {
	m, st := newRootWithOpenChat(t) // chat 1 open and focused
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, UnreadReactionsCount: 1})

	newM, cmd := applyEvent(t, m, st, store.Event{
		Kind:            store.EventReactionsUpdate,
		ChatID:          1,
		MsgID:           500,
		ReactionsUnread: true,
	})
	_ = newM.(ui.RootModel)
	require.NotNil(t, cmd)

	// Invoking the command asks the owner to read the reactions.
	drainMsgs(cmd())
	assert.Equal(t, 1, ownerOf(t, m).reactionsRead)
}

// A reaction delivered as an edit of an already-edited message must be marked
// read like any other when the user is looking at the chat (#199).
func TestRoot_EditWithReaction_OnOpenChat_ReadsReactions(t *testing.T) {
	m, st := newRootWithOpenChat(t) // chat 1 open and focused
	edited := time.Now().Add(-time.Hour)
	st.AppendMessage(domain.Message{ID: 500, ChatID: 1, Text: "fixed typo", EditDate: &edited})

	_, cmd := applyEvent(t, m, st, store.Event{
		Kind: store.EventEditMessage,
		Message: domain.Message{
			ID: 500, ChatID: 1, Text: "fixed typo", EditDate: &edited,
			Reactions:          []domain.Reaction{{Emoji: "👍", Count: 1}},
			HasUnreadReactions: true,
		},
	})
	require.NotNil(t, cmd)

	drainMsgs(cmd())
	assert.Equal(t, 1, ownerOf(t, m).reactionsRead)
}

func TestRoot_OpenChat_ClearsUnreadReactionsOptimistically(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, UnreadReactionsCount: 3})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// The badge is cleared by the owner's command, ahead of its request (#198).
	newM, cmd := m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Alice"})
	_ = newM.(ui.RootModel)
	require.NotNil(t, cmd)
	drainMsgs(cmd())

	c, ok := st.GetChat(1)
	require.True(t, ok)
	assert.Equal(t, 0, c.UnreadReactionsCount, "opening a chat clears its unread reactions")
}

func TestRoot_NewMention_BumpsIndicatorOnOtherChat(t *testing.T) {
	m, st := newRootWithTwoChats(t)
	// Chat 2 is not open; an incoming mention there bumps its indicator.
	newM, _ := applyEvent(t, m, st, store.Event{
		Kind: store.EventNewMessage,
		Message: domain.Message{
			ID: 500, ChatID: 2, Mentioned: true, IsOut: false,
		},
	})
	root := newM.(ui.RootModel)

	c, ok := st.GetChat(2)
	require.True(t, ok)
	assert.Equal(t, 1, c.UnreadMentionsCount)

	var chat2 project.ChatRow
	for _, ch := range root.ChatList().Rows() {
		if ch.ID == 2 {
			chat2 = ch
		}
	}
	assert.Equal(t, 1, chat2.Mentions)
}

func TestRoot_OpenChat_ClearsUnreadMentionsOptimistically(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, UnreadMentionsCount: 3})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	newM, cmd := m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Alice"})
	_ = newM.(ui.RootModel)
	require.NotNil(t, cmd)
	drainMsgs(cmd())

	c, ok := st.GetChat(1)
	require.True(t, ok)
	assert.Equal(t, 0, c.UnreadMentionsCount, "opening a chat clears its unread mentions")
}

func TestRoot_PasteMsg_WhenComposerFocused_InsertsText(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	// enter insert mode → focuses composer
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = newM.(ui.RootModel)
	require.True(t, m.Chat().ComposerFocused())

	newM, _ = m.Update(tea.PasteMsg{Content: "pasted text"})
	m = newM.(ui.RootModel)

	assert.Equal(t, "pasted text", m.Chat().ComposerValue())
}

func TestRoot_PasteMsg_WhenSearchOpen_UpdatesQuery(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
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
	m := ui.NewRootModel(nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// Open a chat — this sets focus to FocusChat.
	newM, _ := m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Alice"})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusChat, m.CurrentFocus())

	// Press Esc in normal mode.
	newM, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
}

func TestRoot_SetTmpDir(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false)
	m.SetTmpDir("/tmp/tele-test")
	assert.Equal(t, "/tmp/tele-test", m.TmpDir())
}

// TestRoot_NewMessageEvent_AlreadyReadElsewhere reproduces issue #88.
// gotd dispatches OtherUpdates (including updateReadHistoryInbox) before NewMessages
// in a getDifference response. A message whose ID is covered by the read pointer
// must not produce a false unread badge when it arrives after the read event.
func TestRoot_NewMessageEvent_AlreadyReadElsewhere(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{
		ID:             2,
		Title:          "Bob",
		ReadInboxMaxID: 100,
		UnreadCount:    0,
	})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	m = newM.(ui.RootModel)

	// EventReadInbox arrives first (gotd OtherUpdates before NewMessages)
	newM, _ = applyEvent(t, m, st, store.Event{
		Kind:      store.EventReadInbox,
		ChatID:    2,
		ReadMaxID: 100,
	})
	m = newM.(ui.RootModel)

	// EventNewMessage for a message already covered by the read pointer
	newM, _ = applyEvent(t, m, st, store.Event{
		Kind:    store.EventNewMessage,
		Message: domain.Message{ID: 99, ChatID: 2, Text: "read elsewhere"},
	})
	root := newM.(ui.RootModel)

	var chat2 project.ChatRow
	for _, c := range root.ChatList().Rows() {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 0, chat2.Unread, "message read elsewhere must not increment unread badge")
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
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", ReadInboxMaxID: 100, UnreadCount: 0})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// Catch-up: new message (already read on another client) arrives before
	// GetDialogs completes and before the read ack (which is dropped).
	newM, _ := applyEvent(t, m, st, store.Event{
		Kind:    store.EventNewMessage,
		Message: domain.Message{ID: 150, ChatID: 2, Text: "read elsewhere"},
	})
	m = newM.(ui.RootModel)

	// GetDialogs completes: server reports the chat as already read.
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", ReadInboxMaxID: 150, UnreadCount: 0})

	// Transition rebuilds the chat list from the authoritative store.
	newM, _ = m.Update(screens.TransitionToMainMsg{})
	root := newM.(ui.RootModel)

	var chat2 project.ChatRow
	for _, c := range root.ChatList().Rows() {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 0, chat2.Unread, "authoritative server read state must clear stale unread badge")
}

func TestRoot_Space_OpensChatMenu_OnChatList(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "A", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false).
		WithScreen(ui.ScreenMain).WithFocus(ui.FocusChatList)
	// Reaching the main screen subscribes the chat list; its first delta fills it.
	m = toMain(t, m)

	nm, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = nm.(ui.RootModel)
	assert.True(t, m.ChatMenuOpen())
}

// TestRoot_RebindChatListConfirmToL_OpensChat reproduces issue #132. "l" is a
// global focus-cycle key, but a chatlist-context override must win over global:
// pressing it opens the chat under the cursor instead of cycling focus.
func TestRoot_RebindChatListConfirmToL_OpensChat(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false).
		WithScreen(ui.ScreenMain).WithFocus(ui.FocusChatList)
	m = toMain(t, m)

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

// Muting goes to the owner as a command; the optimistic write is the owner's,
// so it lands when the command runs rather than inside Update (#198).
func TestRoot_ToggleMute_GoesThroughTheOwner(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "A", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)

	updated, cmd := m.Update(components.ToggleMuteRequest{ChatID: 1, Muted: true})
	rm := updated.(ui.RootModel)
	assert.False(t, rm.ChatMenuOpen(), "menu closes after action")
	require.NotNil(t, cmd, "the request must produce an owner command")
	drainMsgs(cmd())

	c, _ := st.GetChat(1)
	assert.True(t, c.IsMuted, "the owner applied the mute")
}

func TestRoot_MarkUnread_GoesThroughTheOwner(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)

	_, cmd := m.Update(components.ToggleUnreadRequest{ChatID: 1, Unread: true})

	require.NotNil(t, cmd, "the request must produce an owner command")
	drainMsgs(cmd())
	c, _ := st.GetChat(1)
	assert.True(t, c.UnreadMark)
}

func TestRoot_ToggleArchive_GoesThroughTheOwner(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)

	_, cmd := m.Update(components.ToggleArchiveRequest{ChatID: 1, Archived: true})

	require.NotNil(t, cmd, "the request must produce an owner command")
	drainMsgs(cmd())
	c, _ := st.GetChat(1)
	assert.True(t, c.IsArchived)
}

func TestRoot_EventEditMessage_HiddenEdit_DoesNotMarkEdited(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "original"})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// A hidden edit (edit_hide) reaches the root as EventEditMessage carrying an
	// edit date Telegram asks not to show — e.g. a reaction bump. It must not
	// flip the message to "edited" (issue #118).
	at := time.Now()
	newM, _ := applyEvent(t, m, st, store.Event{
		Kind:    store.EventEditMessage,
		Message: domain.Message{ID: 10, ChatID: 1, Text: "original", EditDate: &at, EditHidden: true},
	})
	_ = newM.(ui.RootModel)

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.False(t, msgs[0].ShowsEdited(), "hidden edit must not set the edited marker")
}

func TestRoot_EventEditMessage_HiddenEdit_UpdatesTheText(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "thinking"})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// A bot streaming its reply rewrites one message under hidden edits. The
	// text must reach the chat, the label must not (#269).
	at := time.Now()
	newM, _ := applyEvent(t, m, st, store.Event{
		Kind:    store.EventEditMessage,
		Message: domain.Message{ID: 10, ChatID: 1, Text: "the whole answer", EditDate: &at, EditHidden: true},
	})
	_ = newM.(ui.RootModel)

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Equal(t, "the whole answer", msgs[0].Text)
	assert.False(t, msgs[0].ShowsEdited())
}

func TestRoot_EventEditMessage_HiddenEdit_AppliesReactions(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	st.AppendMessage(domain.Message{ID: 10, ChatID: 1, Text: "original"})
	m := newRoot(st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// In a 1:1 chat an incoming reaction is delivered as a hidden edit
	// (edit_hide) carrying the message's new reactions, not as a separate
	// UpdateMessageReactions. The reactions must be applied so they appear live,
	// while the message must still not be flipped to "edited" (#160, #118).
	at := time.Now()
	newM, _ := applyEvent(t, m, st, store.Event{
		Kind: store.EventEditMessage,
		Message: domain.Message{
			ID: 10, ChatID: 1, Text: "original", EditDate: &at, EditHidden: true,
			Reactions: []domain.Reaction{{Emoji: "👍", Count: 1}},
		},
	})
	_ = newM.(ui.RootModel)

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.False(t, msgs[0].ShowsEdited(), "hidden edit must not set the edited marker")
	require.Len(t, msgs[0].Reactions, 1, "reactions from the hidden edit must be applied")
	assert.Equal(t, "👍", msgs[0].Reactions[0].Emoji)
	assert.Equal(t, 1, msgs[0].Reactions[0].Count)
}

func TestRoot_SearchUsersRequestRunsRPCAndRoutesResult(t *testing.T) {
	st := store.NewMemory()
	m := newRoot(st, 20, false).WithScreen(ui.ScreenMain)
	// Searching is an owner query now (#198).
	ownerOf(t, m).searchResult = []project.ChatRow{
		{ID: 99, Title: "Zoe", IsUser: true},
	}

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
	assert.Equal(t, "zo", ownerOf(t, m).lastSearchQuery)
	require.Len(t, res.Chats, 1)
	assert.Equal(t, int64(99), res.Chats[0].ID)
	assert.Equal(t, 1, res.Serial)
}

// A reaction that lands while the chat is on screen must be marked read, even
// though the message it landed on may be far outside the window.
func TestRoot_ReactionWhileChatOpen_MarksItRead(t *testing.T) {
	m, st := newRootWithOpenChat(t) // chat 1 open and focused
	st.SetMessages(1, []domain.Message{
		{ID: 10, ChatID: 1, Text: "mine", IsOut: true, Date: time.Unix(1, 0)},
	})
	nm, _ := applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)

	_, cmd := applyEvent(t, m, st, store.Event{
		Kind: store.EventReactionsUpdate, ChatID: 1, MsgID: 10,
		Reactions: []domain.Reaction{{Emoji: "👍", Count: 1}}, ReactionsUnread: true,
	})

	require.NotNil(t, cmd, "an arriving reaction on the open chat must be read")
	drainMsgs(cmd())
	assert.Equal(t, 1, ownerOf(t, m).reactionsRead)
}

// A reaction that arrives while the chat pane is not focused is only seen when
// the user looks at it, so that is when it counts as read.
func TestRoot_ReactionArrivingUnfocused_IsReadOnFocus(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.SetMessages(1, []domain.Message{
		{ID: 10, ChatID: 1, Text: "mine", IsOut: true, Date: time.Unix(1, 0)},
	})
	nm, _ := applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel).WithFocus(ui.FocusChatList)

	nm, cmd := applyEvent(t, m, st, store.Event{
		Kind: store.EventReactionsUpdate, ChatID: 1, MsgID: 10,
		Reactions: []domain.Reaction{{Emoji: "👍", Count: 1}}, ReactionsUnread: true,
	})
	m = nm.(ui.RootModel)
	assert.Nil(t, cmd, "nobody is looking at the chat pane yet")

	// Focusing the pane is the moment the reaction is seen.
	out, cmd := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	_ = out
	require.NotNil(t, cmd, "focusing the chat must read the waiting reaction")
	drainMsgs(cmd())
	assert.Equal(t, 1, ownerOf(t, m).reactionsRead)
}
