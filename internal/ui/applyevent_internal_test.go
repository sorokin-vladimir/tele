package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// ownerStub is the in-package twin of the ui_test testOwner: one state and one
// projection registry, queueing the deltas a mutation produces so a test can
// drain them into the model synchronously.
type ownerStub struct {
	state    *state.State
	reg      *project.Registry
	queued   []project.Delta
	incoming []core.Incoming
	typing   []core.Typing

	// focus records what the client told the owner it is showing, so a test can
	// assert that leaving a chat is reported as well as entering one (#192).
	focus []int64

	// calls records the commands the UI issued, so a test can assert on what
	// was asked for rather than on how the result was rendered. err is what
	// every command answers with, standing in for a Telegram refusal.
	calls []cmdCall
	err   error
	// participants is what the mention query answers with.
	participants []domain.ChatMember

	// knownUsers is what KnownUser answers from, fullUsers what GetUser
	// completes with; userErr, when set, is what GetUser fails with instead.
	// Split so a test can pin the gap between the two, which is the whole of
	// the partial profile (#222).
	knownUsers map[int64]domain.User
	fullUsers  map[int64]domain.User
	userErr    error
	// gotUsers records the ids GetUser was asked for.
	gotUsers []int64

	// mediaPaths is what FetchMedia and SaveMedia serve; a slot with no entry
	// answers NotFound, standing in for a download failure. fetched records what
	// the client asked for, invalidated what it asked to drop. mediaErr, when
	// set, is what both answer with instead, standing in for a named refusal.
	mediaPaths  map[mediaKey]string
	mediaErr    error
	fetched     []mediaKey
	invalidated []mediaKey

	// The same three for avatars, which travel their own path: a person and a
	// picture id rather than a message slot (#223).
	avatarPaths        map[avatarKey]string
	avatarsFetched     []avatarKey
	avatarsInvalidated []avatarKey

	// The durable send queue (#193): what the UI submitted, retried, discarded.
	sent      []core.SendRequest
	sentMedia []core.MediaSendRequest
	retried   []string
	discarded []string
}

func (o *ownerStub) SetFocus(chatID int64) { o.focus = append(o.focus, chatID) }

// mediaKey identifies one piece of media the way a client names it.
type mediaKey struct {
	chatID int64
	msgID  int
	slot   domain.MediaSlot
}

// avatarKey identifies one person's picture the way a client names it.
type avatarKey struct {
	userID   int64
	avatarID int64
}

// cmdCall is one command the UI issued through the owner.
type cmdCall struct {
	name   string
	chatID int64
	flag   bool
}

// storeReader lends a bare store the projection's fifth method. The real owner
// composes the store with its send queue (#193); this stub has no queue, and an
// empty one reads the same.
type storeReader struct{ store.Store }

func (storeReader) Outbox(int64) []domain.OutboxEntry { return nil }

func newOwnerStub(st store.Store) *ownerStub {
	o := &ownerStub{state: state.New(st), reg: project.NewRegistry(storeReader{st}), mediaPaths: make(map[mediaKey]string)}
	o.state.OnChange(func(chg state.Change) {
		if chg.Kind == state.ChangeTyping {
			o.typing = append(o.typing, core.Typing{ChatID: chg.ChatID, Label: chg.Typing.Label()})
			return
		}
		o.queued = append(o.queued, o.reg.Refresh()...)
	})
	return o
}

func (o *ownerStub) Subscribe(w project.Window) project.SubID {
	id, ds := o.reg.Subscribe(w)
	o.queued = append(o.queued, ds...)
	return id
}

func (o *ownerStub) MoveWindow(id project.SubID, w project.Window) {
	o.queued = append(o.queued, o.reg.MoveWindow(id, w)...)
}

func (o *ownerStub) Unsubscribe(id project.SubID) { o.reg.Unsubscribe(id) }

func (o *ownerStub) Refresh() { o.queued = append(o.queued, o.reg.Refresh()...) }

// SetMuted mirrors the real owner: it applies the change through state (which
// publishes a delta) and answers with o.err.
func (o *ownerStub) SetMuted(_ context.Context, chatID int64, muted bool) error {
	o.calls = append(o.calls, cmdCall{name: "SetMuted", chatID: chatID, flag: muted})
	if o.err != nil {
		return o.err
	}
	o.state.ApplyMute(chatID, muted)
	return nil
}

func (o *ownerStub) SetArchived(_ context.Context, chatID int64, archived bool) error {
	o.calls = append(o.calls, cmdCall{name: "SetArchived", chatID: chatID, flag: archived})
	if o.err != nil {
		return o.err
	}
	o.state.ApplyArchived(chatID, archived)
	return nil
}

func (o *ownerStub) SetUnreadMark(_ context.Context, chatID int64, unread bool) error {
	o.calls = append(o.calls, cmdCall{name: "SetUnreadMark", chatID: chatID, flag: unread})
	if o.err != nil {
		return o.err
	}
	o.state.ApplyUnreadMark(chatID, unread)
	return nil
}

func (o *ownerStub) SearchContacts(_ context.Context, _ string, _ int) ([]domain.Chat, error) {
	return nil, o.err
}

func (o *ownerStub) GetParticipants(_ context.Context, chatID int64) ([]domain.ChatMember, error) {
	o.calls = append(o.calls, cmdCall{name: "GetParticipants", chatID: chatID})
	return o.participants, o.err
}

// KnownUser mirrors the real owner: it answers from the dialog when the test
// has not seeded a user of its own, so a chat in the store is enough to open a
// profile on.
func (o *ownerStub) KnownUser(userID int64) (domain.User, bool) {
	if u, ok := o.knownUsers[userID]; ok {
		return u, true
	}
	if chat, ok := o.state.Store().GetChat(userID); ok && chat.Peer.IsUser() {
		return domain.User{ID: userID, FirstName: chat.Title, Online: chat.Online, IsBot: chat.IsBot, IsContact: chat.IsContact}, true
	}
	return domain.User{}, false
}

func (o *ownerStub) GetUser(_ context.Context, userID int64) (domain.User, error) {
	o.gotUsers = append(o.gotUsers, userID)
	if o.userErr != nil {
		return domain.User{}, o.userErr
	}
	if u, ok := o.fullUsers[userID]; ok {
		return u, nil
	}
	return domain.User{ID: userID}, nil
}

func (o *ownerStub) FetchMedia(_ context.Context, chatID int64, msgID int, slot domain.MediaSlot) (string, error) {
	key := mediaKey{chatID, msgID, slot}
	o.fetched = append(o.fetched, key)
	if o.mediaErr != nil {
		return "", o.mediaErr
	}
	p, ok := o.mediaPaths[key]
	if !ok {
		return "", &telerr.Error{Kind: telerr.NotFound}
	}
	return p, nil
}

// SaveMedia copies the registered file into destDir, the way the real owner
// streams it there.
func (o *ownerStub) SaveMedia(_ context.Context, chatID int64, msgID int, slot domain.MediaSlot, destDir string) (string, error) {
	if o.mediaErr != nil {
		return "", o.mediaErr
	}
	src, ok := o.mediaPaths[mediaKey{chatID, msgID, slot}]
	if !ok {
		return "", &telerr.Error{Kind: telerr.NotFound}
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(destDir, filepath.Base(src))
	if err := os.WriteFile(dst, data, 0600); err != nil {
		return "", err
	}
	return dst, nil
}

func (o *ownerStub) InvalidateMedia(chatID int64, msgID int, slot domain.MediaSlot) {
	o.invalidated = append(o.invalidated, mediaKey{chatID, msgID, slot})
}

// FetchAvatar serves avatarPaths and records what was asked for, so a test can
// assert that a picture was (or was not) requested for a person (#223).
func (o *ownerStub) FetchAvatar(_ context.Context, userID, avatarID int64) (string, error) {
	key := avatarKey{userID, avatarID}
	o.avatarsFetched = append(o.avatarsFetched, key)
	p, ok := o.avatarPaths[key]
	if !ok {
		return "", &telerr.Error{Kind: telerr.NotFound}
	}
	return p, nil
}

func (o *ownerStub) InvalidateAvatar(userID, avatarID int64) {
	o.avatarsInvalidated = append(o.avatarsInvalidated, avatarKey{userID, avatarID})
}

func (o *ownerStub) SetTyping(_ context.Context, chatID int64, _ domain.TypingAction) error {
	o.calls = append(o.calls, cmdCall{name: "SetTyping", chatID: chatID})
	return o.err
}

func (o *ownerStub) SaveDraft(_ context.Context, chatID int64, text string) error {
	o.calls = append(o.calls, cmdCall{name: "SaveDraft", chatID: chatID})
	o.state.ApplyDraft(chatID, text)
	return o.err
}

// The durable send queue (#193). The stub records submissions rather than
// draining them: what the UI is responsible for is handing the message over.
func (o *ownerStub) Send(_ context.Context, req core.SendRequest) error {
	if o.err != nil {
		return o.err
	}
	o.sent = append(o.sent, req)
	return nil
}

// Media is submitted the same way: the client names paths and hands them over,
// and everything protocol-shaped happens on the other side (#195).
func (o *ownerStub) SendMedia(_ context.Context, req core.MediaSendRequest) error {
	if o.err != nil {
		return o.err
	}
	o.sentMedia = append(o.sentMedia, req)
	return nil
}

func (o *ownerStub) RetryOutbox(ref string) error {
	o.retried = append(o.retried, ref)
	return o.err
}

func (o *ownerStub) DiscardOutbox(ref string) error {
	o.discarded = append(o.discarded, ref)
	return o.err
}

func (o *ownerStub) Forward(_ context.Context, fromChatID int64, to domain.Peer, _ []int, _ string) error {
	o.calls = append(o.calls, cmdCall{name: "Forward", chatID: fromChatID})
	if o.err != nil {
		return o.err
	}
	o.state.Store().BumpChatLastMessage(to.ID, domain.Message{ChatID: to.ID, IsOut: true, Date: time.Now()})
	o.queued = append(o.queued, o.reg.Refresh()...)
	return nil
}

func (o *ownerStub) SendReaction(_ context.Context, chatID int64, msgID int, emoji string) error {
	o.calls = append(o.calls, cmdCall{name: "SendReaction", chatID: chatID})
	if o.err != nil {
		return o.err
	}
	o.state.ApplyReactions(chatID, msgID,
		[]domain.Reaction{{Emoji: emoji, Count: 1, IsChosen: true}}, false)
	return nil
}

func (o *ownerStub) DeleteMessages(_ context.Context, chatID int64, msgIDs []int, _ bool) error {
	o.calls = append(o.calls, cmdCall{name: "DeleteMessages", chatID: chatID})
	removed := make([]domain.Message, 0, len(msgIDs))
	for _, m := range o.state.Store().Messages(chatID) {
		for _, id := range msgIDs {
			if m.ID == id {
				removed = append(removed, m)
			}
		}
	}
	o.state.ApplyDelete(chatID, msgIDs)
	if o.err != nil {
		for _, m := range removed {
			o.state.ApplyRestore(m)
		}
		return o.err
	}
	return nil
}

func (o *ownerStub) EditMessage(_ context.Context, chatID int64, msgID int, text string, entities []domain.MessageEntity) error {
	o.calls = append(o.calls, cmdCall{name: "EditMessage", chatID: chatID})
	var prev domain.Message
	found := false
	for _, m := range o.state.Store().Messages(chatID) {
		if m.ID == msgID {
			prev, found = m, true
			break
		}
	}
	if !found {
		return &telerr.Error{Kind: telerr.NotFound}
	}
	if o.err != nil {
		return o.err
	}
	edited := prev
	edited.Text = text
	edited.Entities = entities
	now := time.Now()
	edited.EditDate = &now
	o.state.ApplyEdit(edited)
	return nil
}

func (o *ownerStub) ReadReactions(_ context.Context, chatID int64) error {
	o.calls = append(o.calls, cmdCall{name: "ReadReactions", chatID: chatID})
	if o.err != nil {
		return o.err
	}
	o.state.ApplyReactionsRead(chatID)
	return nil
}

func (o *ownerStub) ReadMentions(_ context.Context, chatID int64) error {
	o.calls = append(o.calls, cmdCall{name: "ReadMentions", chatID: chatID})
	if o.err != nil {
		return o.err
	}
	o.state.ApplyMentionsRead(chatID)
	return nil
}

func (o *ownerStub) MarkRead(_ context.Context, chatID int64, maxID int) error {
	o.calls = append(o.calls, cmdCall{name: "MarkRead", chatID: chatID})
	if o.err != nil {
		return o.err
	}
	if maxID == 0 {
		o.state.ApplyChatRead(chatID)
		return nil
	}
	o.state.ApplyReadInbox(chatID, maxID)
	return nil
}

func (o *ownerStub) AddToFolder(_ context.Context, filterID int, chatID int64, add bool) error {
	o.calls = append(o.calls, cmdCall{name: "AddToFolder", chatID: chatID, flag: add})
	if o.err != nil {
		return o.err
	}
	o.state.ApplyFolderMembership(filterID, chatID, add)
	return nil
}

func (o *ownerStub) drain(m RootModel) (tea.Model, tea.Cmd) {
	var model tea.Model = m
	var cmd tea.Cmd
	for len(o.queued) > 0 || len(o.incoming) > 0 || len(o.typing) > 0 {
		deltas, events, typing := o.queued, o.incoming, o.typing
		o.queued, o.incoming, o.typing = nil, nil, nil
		for _, d := range deltas {
			model, cmd = model.Update(d)
		}
		for _, in := range events {
			model, cmd = model.Update(in)
		}
		for _, tp := range typing {
			model, cmd = model.Update(tp)
		}
	}
	return model, cmd
}

// newRootInternal builds a model wired to the stub owner, as app.Run wires the
// real one.
func newRootInternal(st store.Store, historyLimit int) RootModel {
	m := NewRootModel(st, historyLimit, false)
	if st != nil {
		m = m.WithOwner(newOwnerStub(st))
	}
	return m
}

// applyEventInternal is the in-package twin of applyEvent in the ui_test
// package: it applies a raw Telegram event to domain state and drains the
// resulting projection deltas into the model, as the owner's update loop and the
// delta pump do in production.
func applyEventInternal(t *testing.T, m RootModel, st store.Store, evt store.Event) (tea.Model, tea.Cmd) {
	t.Helper()
	o, ok := m.owner.(*ownerStub)
	if !ok {
		state.Apply(state.New(st), evt)
		return m, nil
	}
	chg, applied := state.Apply(o.state, evt)
	if applied && chg.Kind == state.ChangeNewMessage && !chg.Message.IsOut &&
		chg.ChatID != m.currentChatID {
		// The flash, and only the flash. Whether the event also deserves a toast
		// is a core.Notification the owner decides; no shim here reproduces that
		// judgement any more (#192).
		o.incoming = append(o.incoming, core.Incoming{ChatID: chg.ChatID})
	}
	return o.drain(m)
}

// A WEBP sticker only decodes if the decoder is registered, and it is
// registered by a blank import the compiler will not miss if it is dropped:
// image.Decode simply answers "unknown format" and stickers silently stop
// rendering (#196).
func TestFetchStickerCmd_DecodesAWebpFile(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	data, err := os.ReadFile(filepath.Join("testdata", "sticker.webp"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "sticker.webp")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	o.mediaPaths[mediaKey{1, 5, domain.DocFull}] = path

	msg := fetchStickerCmd(context.Background(), o, 1, 5, 11)()

	ready, ok := msg.(PhotoReadyMsg)
	if !ok {
		t.Fatalf("expected a PhotoReadyMsg, got %T", msg)
	}
	if ready.PhotoID != 11 {
		t.Fatalf("PhotoID = %d, want 11", ready.PhotoID)
	}
	if ready.Image == nil {
		t.Fatal("sticker did not decode: is the WEBP decoder registered?")
	}
}

// A cached file that will not decode must not wedge: the entry is dropped so
// the next repaint downloads it again.
func TestFetchPhotoCmd_InvalidatesAnUndecodableFile(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	path := filepath.Join(t.TempDir(), "broken")
	if err := os.WriteFile(path, []byte("not an image"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	o.mediaPaths[mediaKey{1, 5, domain.PhotoThumb}] = path

	msg := fetchPhotoCmd(context.Background(), o, 1, 5, 9)()

	if msg != nil {
		t.Fatalf("expected no message, got %T", msg)
	}
	if len(o.invalidated) != 1 || o.invalidated[0] != (mediaKey{1, 5, domain.PhotoThumb}) {
		t.Fatalf("expected the entry to be invalidated, got %v", o.invalidated)
	}
}

// An expired file reference on a preview nobody asked for is not news: the
// owner refreshes it and logs whatever it could not repair, and the window
// fetches again once the fresh reference lands. Opening a chat whose messages
// came back from disk raised one of these per photo, per video poster and per
// eagerly prefetched full-size photo.
func TestFetchPhotoCmd_AnExpiredReferenceIsNotReportedToTheUser(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	o.mediaErr = &telerr.Error{Kind: telerr.StaleReference}

	msg := fetchPhotoCmd(context.Background(), o, 1, 5, 9)()

	if msg != nil {
		t.Fatalf("expected no status message, got %T: %v", msg, msg)
	}
}

func TestFetchVideoThumbCmd_AnExpiredReferenceIsNotReportedToTheUser(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	o.mediaErr = &telerr.Error{Kind: telerr.StaleReference}

	msg := fetchVideoThumbCmd(context.Background(), o, 1, 5, 11, false)()

	if msg != nil {
		t.Fatalf("expected no status message, got %T: %v", msg, msg)
	}
}

// Only the expiry is swallowed. Anything else still reaches the status bar,
// which is the difference between a quiet self-healing case and a silent
// client.
func TestFetchPhotoCmd_OtherFailuresAreStillReported(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	o.mediaErr = &telerr.Error{Kind: telerr.Forbidden}

	msg := fetchPhotoCmd(context.Background(), o, 1, 5, 9)()

	if _, ok := msg.(StatusErrMsg); !ok {
		t.Fatalf("expected a StatusErrMsg, got %T", msg)
	}
}

// The eager full-quality prefetch runs for every photo in the window without
// anyone asking, so it is as silent as the thumbnail fetch.
func TestSaveFullPhotoCmd_BackgroundPrefetchDoesNotReportAnExpiredReference(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	o.mediaErr = &telerr.Error{Kind: telerr.StaleReference}

	msg := saveFullPhotoCmd(context.Background(), o, 1, 5, 9, t.TempDir(), true)()

	if msg != nil {
		t.Fatalf("expected no status message, got %T: %v", msg, msg)
	}
}

// In the viewer the same failure is worth saying: the user is looking at the
// photo and would otherwise wonder why it stays at preview quality.
func TestSaveFullPhotoCmd_TheViewerReportsAnExpiredReference(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	o.mediaErr = &telerr.Error{Kind: telerr.StaleReference}

	msg := saveFullPhotoCmd(context.Background(), o, 1, 5, 9, t.TempDir(), false)()

	if _, ok := msg.(StatusErrMsg); !ok {
		t.Fatalf("expected a StatusErrMsg, got %T", msg)
	}
}

func TestSaveFileCmd_ReportsTheSavedPath(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	src := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(src, []byte("video"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	o.mediaPaths[mediaKey{1, 5, domain.DocFull}] = src
	dest := t.TempDir()

	msg := saveFileCmd(context.Background(), o, 1, 5, domain.DocFull, dest, 0)()

	done, ok := msg.(fileDownloadDoneMsg)
	if !ok {
		t.Fatalf("expected a fileDownloadDoneMsg, got %T", msg)
	}
	if done.sev != components.SeverityInfo {
		t.Fatalf("severity = %v, want info", done.sev)
	}
	if !strings.Contains(done.text, "Saved to ") || !strings.Contains(done.text, "clip.mp4") {
		t.Fatalf("text = %q, want it to name the saved file", done.text)
	}
}

func TestSaveFileCmd_ReportsAFailureAsAnError(t *testing.T) {
	o := newOwnerStub(store.NewMemory()) // no media registered: SaveMedia answers NotFound

	msg := saveFileCmd(context.Background(), o, 1, 5, domain.DocFull, t.TempDir(), 0)()

	done, ok := msg.(fileDownloadDoneMsg)
	if !ok {
		t.Fatalf("expected a fileDownloadDoneMsg, got %T", msg)
	}
	if done.sev == components.SeverityInfo {
		t.Fatal("a failed save must not report success")
	}
	if done.text == "" {
		t.Fatal("a failed save must say something")
	}
}

func TestOpenDocumentCmd_LaunchesTheSavedFile(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	src := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(src, []byte("video"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	o.mediaPaths[mediaKey{1, 5, domain.DocFull}] = src
	var opened string
	restore := SetOpenPathForTest(func(p string) { opened = p })
	defer restore()

	msg := openDocumentCmd(context.Background(), o, 1, 5, t.TempDir(), 0)()

	done, ok := msg.(documentOpenDoneMsg)
	if !ok {
		t.Fatalf("expected a documentOpenDoneMsg, got %T", msg)
	}
	if done.errText != "" {
		t.Fatalf("unexpected error: %q", done.errText)
	}
	if !strings.Contains(opened, "clip.mp4") {
		t.Fatalf("opened %q, want the saved clip", opened)
	}
}
