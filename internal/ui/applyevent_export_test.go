package ui_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// testOwner stands in for core.Owner (#194). It holds the same pieces the real
// owner does — one state, one projection registry — and queues the deltas a
// mutation produces instead of pushing them down a channel, so a test can drain
// them into the model synchronously.
type testOwner struct {
	state  *state.State
	reg    *project.Registry
	queued []project.Delta
	// incoming records the events the real owner would publish alongside the
	// deltas: a message arriving in a chat the client is not showing.
	incoming []core.Incoming
	typing   []core.Typing
	// focus records what the client told the owner it is showing, so a test can
	// assert that leaving a chat is reported as well as entering one (#192).
	focus []int64
	// moves records the windows the client asked for, so a test can assert that
	// scrolling repositions a window rather than fetching anything itself.
	moves []project.Window
	// cmdErr is what every command answers with, standing in for a refusal.
	cmdErr error
	// knownUsers is what KnownUser answers from, fullUsers what GetUser
	// completes with, userErr what GetUser fails with instead. Split so a test
	// can pin the gap between the two, which is the partial profile (#222).
	knownUsers map[int64]domain.User
	fullUsers  map[int64]domain.User
	userErr    error
	gotUsers   []int64
	// reactionsRead and mentionsRead count the badge-clearing commands, which
	// used to be counted on the mock tg.Client before they became owner
	// commands (#198).
	reactionsRead int
	mentionsRead  int
	// forward records the last Forward call, replacing what used to be asserted
	// on the mock tg.Client (#198).
	forwardFrom    int64
	forwardTo      domain.Peer
	forwardIDs     []int
	forwardComment string
	savedDrafts    []ownerDraft
	// The durable send queue (#193): what the UI submitted, retried, discarded,
	// and the context it submitted under.
	sent        []core.SendRequest
	sentMedia   []core.MediaSendRequest
	retried     []string
	discarded   []string
	lastSendCtx context.Context
	// searchResult and participants are what the queries answer with;
	// lastSearchQuery records what was asked.
	searchResult    []domain.Chat
	participants    []domain.ChatMember
	lastSearchQuery string
	// mediaPaths is what FetchMedia and SaveMedia serve; a slot with no entry
	// answers NotFound, standing in for a download failure. fetched records what
	// the client asked for, invalidated what it asked to drop.
	mediaPaths  map[mediaPathKey]string
	fetched     []mediaPathKey
	invalidated []mediaPathKey
	// The same three for avatars, named by a person and a picture rather than
	// by a message slot (#223).
	avatarPaths        map[avatarPathKey]string
	avatarsFetched     []avatarPathKey
	avatarsInvalidated []avatarPathKey
}

// avatarPathKey identifies one person's picture the way a client names it.
type avatarPathKey struct {
	userID   int64
	avatarID int64
}

// storeReader lends a bare store the projection's fifth method. The real owner
// composes the store with its send queue (#193); this stub has no queue, and an
// empty one reads the same.
type storeReader struct{ store.Store }

func (storeReader) Outbox(int64) []domain.OutboxEntry { return nil }

func newTestOwner(st store.Store) *testOwner {
	o := &testOwner{
		state:       state.New(st),
		reg:         project.NewRegistry(storeReader{st}),
		mediaPaths:  make(map[mediaPathKey]string),
		avatarPaths: make(map[avatarPathKey]string),
	}
	o.state.OnChange(func(chg state.Change) {
		if chg.Kind == state.ChangeTyping {
			o.typing = append(o.typing, core.Typing{ChatID: chg.ChatID, Label: chg.Typing.Label()})
			return
		}
		o.queued = append(o.queued, o.reg.Refresh()...)
	})
	return o
}

func (o *testOwner) Subscribe(w project.Window) project.SubID {
	id, ds := o.reg.Subscribe(w)
	o.queued = append(o.queued, ds...)
	return id
}

func (o *testOwner) MoveWindow(id project.SubID, w project.Window) {
	o.moves = append(o.moves, w)
	o.queued = append(o.queued, o.reg.MoveWindow(id, w)...)
}

// lastChatWindow returns the most recent chat window the client asked for.
func (o *testOwner) lastChatWindow() (project.ChatWindow, bool) {
	for i := len(o.moves) - 1; i >= 0; i-- {
		if w, ok := o.moves[i].(project.ChatWindow); ok {
			return w, true
		}
	}
	return project.ChatWindow{}, false
}

// lastChatListWindow returns the most recent chatlist window the client asked for.
func (o *testOwner) lastChatListWindow() (project.ChatListWindow, bool) {
	for i := len(o.moves) - 1; i >= 0; i-- {
		if w, ok := o.moves[i].(project.ChatListWindow); ok {
			return w, true
		}
	}
	return project.ChatListWindow{}, false
}

func (o *testOwner) Unsubscribe(id project.SubID) { o.reg.Unsubscribe(id) }

func (o *testOwner) SetFocus(chatID int64) { o.focus = append(o.focus, chatID) }

func (o *testOwner) Refresh() { o.queued = append(o.queued, o.reg.Refresh()...) }

// SetMuted mirrors the real owner: the change is applied through state, which
// publishes a delta, and cmdErr stands in for a Telegram refusal.
func (o *testOwner) SetMuted(_ context.Context, chatID int64, muted bool) error {
	if o.cmdErr != nil {
		return o.cmdErr
	}
	o.state.ApplyMute(chatID, muted)
	return nil
}

func (o *testOwner) SetArchived(_ context.Context, chatID int64, archived bool) error {
	if o.cmdErr != nil {
		return o.cmdErr
	}
	o.state.ApplyArchived(chatID, archived)
	return nil
}

func (o *testOwner) SetUnreadMark(_ context.Context, chatID int64, unread bool) error {
	if o.cmdErr != nil {
		return o.cmdErr
	}
	o.state.ApplyUnreadMark(chatID, unread)
	return nil
}

func (o *testOwner) SearchContacts(_ context.Context, q string, _ int) ([]domain.Chat, error) {
	o.lastSearchQuery = q
	return o.searchResult, o.cmdErr
}

func (o *testOwner) GetParticipants(_ context.Context, _ int64) ([]domain.ChatMember, error) {
	return o.participants, o.cmdErr
}

// KnownUser answers from the dialog, like the real owner, so a chat in the
// store is enough for a test to open a profile on. knownUsers overrides it for
// a person the store holds no chat for.
func (o *testOwner) KnownUser(userID int64) (domain.User, bool) {
	if u, ok := o.knownUsers[userID]; ok {
		return u, true
	}
	if chat, ok := o.state.Store().GetChat(userID); ok && chat.Peer.IsUser() {
		return domain.User{ID: userID, FirstName: chat.Title, Online: chat.Online, IsBot: chat.IsBot, IsContact: chat.IsContact}, true
	}
	return domain.User{}, false
}

func (o *testOwner) GetUser(_ context.Context, userID int64) (domain.User, error) {
	o.gotUsers = append(o.gotUsers, userID)
	if o.userErr != nil {
		return domain.User{}, o.userErr
	}
	if u, ok := o.fullUsers[userID]; ok {
		return u, nil
	}
	return domain.User{ID: userID}, nil
}

// mediaPathKey identifies one piece of media the way a client names it.
type mediaPathKey struct {
	chatID int64
	msgID  int
	slot   domain.MediaSlot
}

func (o *testOwner) FetchMedia(_ context.Context, chatID int64, msgID int, slot domain.MediaSlot) (string, error) {
	key := mediaPathKey{chatID, msgID, slot}
	o.fetched = append(o.fetched, key)
	p, ok := o.mediaPaths[key]
	if !ok {
		return "", &telerr.Error{Kind: telerr.NotFound}
	}
	return p, nil
}

// SaveMedia copies the registered file into destDir, the way the real owner
// streams it there.
func (o *testOwner) SaveMedia(_ context.Context, chatID int64, msgID int, slot domain.MediaSlot, destDir string) (string, error) {
	src, ok := o.mediaPaths[mediaPathKey{chatID, msgID, slot}]
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

func (o *testOwner) InvalidateMedia(chatID int64, msgID int, slot domain.MediaSlot) {
	o.invalidated = append(o.invalidated, mediaPathKey{chatID, msgID, slot})
}

// FetchAvatar serves avatarPaths and records the request, so a test can assert
// which person's picture was asked for (#223).
func (o *testOwner) FetchAvatar(_ context.Context, userID, avatarID int64) (string, error) {
	key := avatarPathKey{userID, avatarID}
	o.avatarsFetched = append(o.avatarsFetched, key)
	p, ok := o.avatarPaths[key]
	if !ok {
		return "", &telerr.Error{Kind: telerr.NotFound}
	}
	return p, nil
}

func (o *testOwner) InvalidateAvatar(userID, avatarID int64) {
	o.avatarsInvalidated = append(o.avatarsInvalidated, avatarPathKey{userID, avatarID})
}

func (o *testOwner) SetTyping(_ context.Context, _ int64, _ domain.TypingAction) error {
	return o.cmdErr
}

func (o *testOwner) SaveDraft(_ context.Context, chatID int64, text string) error {
	o.savedDrafts = append(o.savedDrafts, ownerDraft{chatID: chatID, text: text})
	o.state.ApplyDraft(chatID, text)
	return o.cmdErr
}

// The durable send queue (#193). The double records submissions rather than
// draining them: what the UI is responsible for is handing the message over.
func (o *testOwner) Send(ctx context.Context, req core.SendRequest) error {
	o.lastSendCtx = ctx
	if o.cmdErr != nil {
		return o.cmdErr
	}
	o.sent = append(o.sent, req)
	return nil
}

// Media is submitted the same way: the client names paths and hands them over.
// Everything that used to happen in the UI — upload, album assembly, InputMedia
// — is on the other side of this call now (#195).
func (o *testOwner) SendMedia(ctx context.Context, req core.MediaSendRequest) error {
	o.lastSendCtx = ctx
	if o.cmdErr != nil {
		return o.cmdErr
	}
	o.sentMedia = append(o.sentMedia, req)
	return nil
}

func (o *testOwner) RetryOutbox(ref string) error {
	o.retried = append(o.retried, ref)
	return o.cmdErr
}

func (o *testOwner) DiscardOutbox(ref string) error {
	o.discarded = append(o.discarded, ref)
	return o.cmdErr
}

// ownerDraft is one SaveDraft the UI issued, replacing what used to be recorded
// on the mock tg.Client (#198).
type ownerDraft struct {
	chatID int64
	text   string
}

func (o *testOwner) Forward(_ context.Context, fromChatID int64, to domain.Peer, msgIDs []int, comment string) error {
	o.forwardFrom, o.forwardTo, o.forwardIDs, o.forwardComment = fromChatID, to, msgIDs, comment
	if o.cmdErr != nil {
		return o.cmdErr
	}
	preview := domain.Message{ChatID: to.ID, IsOut: true, Date: time.Now()}
	if len(msgIDs) > 0 {
		if src, ok := o.messageByID(fromChatID, msgIDs[0]); ok {
			preview.Text = src.Text
		}
	}
	o.state.Store().BumpChatLastMessage(to.ID, preview)
	o.queued = append(o.queued, o.reg.Refresh()...)
	return nil
}

func (o *testOwner) SendReaction(_ context.Context, chatID int64, msgID int, emoji string) error {
	msg, ok := o.messageByID(chatID, msgID)
	if !ok {
		return &telerr.Error{Kind: telerr.NotFound}
	}
	prev := make([]domain.Reaction, len(msg.Reactions))
	copy(prev, msg.Reactions)
	if o.cmdErr != nil {
		return o.cmdErr
	}
	next := append([]domain.Reaction{}, prev...)
	next = append(next, domain.Reaction{Emoji: emoji, Count: 1, IsChosen: true})
	o.state.ApplyReactions(chatID, msgID, next, false)
	return nil
}

func (o *testOwner) DeleteMessages(_ context.Context, chatID int64, msgIDs []int, _ bool) error {
	removed := make([]domain.Message, 0, len(msgIDs))
	for _, id := range msgIDs {
		if m, ok := o.messageByID(chatID, id); ok {
			removed = append(removed, m)
		}
	}
	o.state.ApplyDelete(chatID, msgIDs)
	if o.cmdErr != nil {
		for _, m := range removed {
			o.state.ApplyRestore(m)
		}
		return o.cmdErr
	}
	return nil
}

func (o *testOwner) EditMessage(_ context.Context, chatID int64, msgID int, text string, entities []domain.MessageEntity) error {
	prev, ok := o.messageByID(chatID, msgID)
	if !ok {
		return &telerr.Error{Kind: telerr.NotFound}
	}
	if o.cmdErr != nil {
		return o.cmdErr
	}
	edited := prev
	edited.Text = text
	edited.Entities = entities
	now := time.Now()
	edited.EditDate = &now
	o.state.ApplyEdit(edited)
	return nil
}

// messageByID mirrors the owner's helper for the commands that need the
// pre-change value.
func (o *testOwner) messageByID(chatID int64, msgID int) (domain.Message, bool) {
	for _, m := range o.state.Store().Messages(chatID) {
		if m.ID == msgID {
			return m, true
		}
	}
	return domain.Message{}, false
}

func (o *testOwner) ReadReactions(_ context.Context, chatID int64) error {
	o.reactionsRead++
	if o.cmdErr != nil {
		return o.cmdErr
	}
	o.state.ApplyReactionsRead(chatID)
	return nil
}

func (o *testOwner) ReadMentions(_ context.Context, chatID int64) error {
	o.mentionsRead++
	if o.cmdErr != nil {
		return o.cmdErr
	}
	o.state.ApplyMentionsRead(chatID)
	return nil
}

// ownerOf returns the testOwner a model was built with, for asserting on the
// commands the UI issued.
func ownerOf(t *testing.T, m ui.RootModel) *testOwner {
	t.Helper()
	o, ok := m.Owner().(*testOwner)
	if !ok {
		t.Fatalf("model has no testOwner attached, got %T", m.Owner())
	}
	return o
}

func (o *testOwner) MarkRead(_ context.Context, chatID int64, maxID int) error {
	if o.cmdErr != nil {
		return o.cmdErr
	}
	if maxID == 0 {
		o.state.ApplyChatRead(chatID)
		return nil
	}
	o.state.ApplyReadInbox(chatID, maxID)
	return nil
}

func (o *testOwner) AddToFolder(_ context.Context, filterID int, chatID int64, add bool) error {
	if o.cmdErr != nil {
		return o.cmdErr
	}
	o.state.ApplyFolderMembership(filterID, chatID, add)
	return nil
}

// drain feeds every queued delta and event into the model, in the order the
// bubbletea program would receive them, and returns the last command.
func (o *testOwner) drain(m ui.RootModel) (tea.Model, tea.Cmd) {
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

// newRoot builds a model wired to a stand-in owner, the way app.Run wires the
// real one. Tests that pass no store get no owner, matching a model that has not
// reached the main screen.
func newRoot(st store.Store, historyLimit int, verbose bool) ui.RootModel {
	m := ui.NewRootModel(st, historyLimit, verbose)
	if st != nil {
		m = m.WithOwner(newTestOwner(st))
	}
	return m
}

// applyEvent stands in for the owner's update loop: it applies a raw Telegram
// event to domain state and drains the resulting projection deltas into the
// model, exactly as core.Owner.RunUpdates plus the delta pump do in production.
//
// A no-op event (a read receipt that does not advance, an unchanged presence)
// produces no change and no delta, which is also what the owner does.
func applyEvent(t *testing.T, m ui.RootModel, st store.Store, evt store.Event) (tea.Model, tea.Cmd) {
	t.Helper()
	o, ok := m.Owner().(*testOwner)
	if !ok {
		// No owner attached: apply so the store advances, but nothing is
		// rendered — the same as a client that subscribed to nothing.
		state.Apply(state.New(st), evt)
		return m, nil
	}
	chg, applied := state.Apply(o.state, evt)
	if applied && chg.Kind == state.ChangeNewMessage && !chg.Message.IsOut &&
		chg.ChatID != m.CurrentChatID() {
		// The flash, and only the flash. Whether the event also deserves a toast
		// is a core.Notification the owner decides; no shim here reproduces that
		// judgement any more (#192).
		o.incoming = append(o.incoming, core.Incoming{ChatID: chg.ChatID})
	}
	return o.drain(m)
}

// openChat opens a chat and drains the subscription's first delta, which is
// always a full Reset. In production that round trip is the bubbletea program
// delivering the owner's reply; a test has to make it happen itself.
func openChat(t testing.TB, m ui.RootModel, chatID int64, title string) ui.RootModel {
	t.Helper()
	next, _ := m.Update(screens.OpenChatMsg{ChatID: chatID, Title: title})
	m = next.(ui.RootModel)
	o, ok := m.Owner().(*testOwner)
	if !ok {
		return m
	}
	drained, _ := o.drain(m)
	return drained.(ui.RootModel)
}

// drainOwner feeds whatever the owner has queued into the model. Anything that
// subscribes or moves a window queues a delta the bubbletea program would
// deliver; a test has to deliver it itself.
func drainOwner(t testing.TB, m ui.RootModel) ui.RootModel {
	t.Helper()
	o, ok := m.Owner().(*testOwner)
	if !ok {
		return m
	}
	drained, _ := o.drain(m)
	return drained.(ui.RootModel)
}

// toMain reaches the main screen, where the chat list first has a size and
// subscribes, and drains the subscription's opening Reset.
func toMain(t testing.TB, m ui.RootModel) ui.RootModel {
	t.Helper()
	next, _ := m.Update(screens.TransitionToMainMsg{})
	return drainOwner(t, next.(ui.RootModel))
}

// applyHistory stands in for a history backfill: it commits the store's current
// messages for a chat through state and drains the resulting chat:<id> delta
// into the model, the way core.Owner.backfill does in production. It replaces
// the old ChatHistoryMsg, which was the client applying a network reply itself.
func applyHistory(t testing.TB, m ui.RootModel, st store.Store, chatID int64) (tea.Model, tea.Cmd) {
	t.Helper()
	o, ok := m.Owner().(*testOwner)
	if !ok {
		return m, nil
	}
	o.state.ApplyHistory(chatID, st.Messages(chatID))
	return o.drain(m)
}
