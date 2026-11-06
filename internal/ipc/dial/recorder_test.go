package dial_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ipc/serve"
	"github.com/sorokin-vladimir/tele/internal/priority"
)

// received is one call as the serving side's surface saw it.
type received struct {
	method string
	args   []any
}

// recorder is a surface that tells what it was asked, and answers with what
// the test gave it: results by method, in the order the method returns them,
// and an error by method.
type recorder struct {
	mu      sync.Mutex
	results map[string][]any
	errs    map[string]error
	calls   chan received
	// block holds the calls named in it until their context ends.
	block  map[string]bool
	stream chan any
	// inBackground is, for a call that records it, whether the context it was
	// made with marked it as background work.
	inBackground map[string]bool
}

// wasBackground reports whether the last call of method came marked as
// background work.
func (r *recorder) wasBackground(method string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inBackground[method]
}

func newRecorder() *recorder {
	return &recorder{
		results: map[string][]any{},
		errs:    map[string]error{},
		calls:   make(chan received, 16),
		block:   map[string]bool{},
		stream:  make(chan any, core.StreamSize),

		inBackground: map[string]bool{},
	}
}

var _ serve.Attachment = (*recorder)(nil)

func (r *recorder) SubscribeAs(id project.SubID, w project.Window) error {
	return r.took("SubscribeAs", id, w)
}
func (r *recorder) Stream() <-chan any { return r.stream }
func (r *recorder) Detach()            { _ = r.took("Detach") }
func (r *recorder) Tell(ev any)        { r.stream <- ev }

// held waits, for a method the test blocked, until ctx ends, and tells the
// test that it did.
func (r *recorder) held(ctx context.Context, method string) {
	r.mu.Lock()
	block := r.block[method]
	r.mu.Unlock()
	if block {
		<-ctx.Done()
		r.calls <- received{method: method + " ended", args: []any{ctx.Err()}}
	}
}

// next is the next call the surface received.
func (r *recorder) next(t *testing.T) received {
	t.Helper()
	select {
	case c := <-r.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("the surface was never called")
		return received{}
	}
}

func (r *recorder) took(method string, args ...any) error {
	r.calls <- received{method: method, args: args}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.errs[method]
}

func result[T any](r *recorder, method string, i int) T {
	r.mu.Lock()
	defer r.mu.Unlock()
	var zero T
	if i >= len(r.results[method]) {
		return zero
	}
	v, _ := r.results[method][i].(T)
	return v
}

var _ core.Surface = (*recorder)(nil)

func (r *recorder) Subscribe(w project.Window) project.SubID {
	_ = r.took("Subscribe", w)
	return result[project.SubID](r, "Subscribe", 0)
}
func (r *recorder) MoveWindow(id project.SubID, w project.Window) { _ = r.took("MoveWindow", id, w) }
func (r *recorder) Unsubscribe(id project.SubID)                  { _ = r.took("Unsubscribe", id) }
func (r *recorder) LogOutPreview() core.LogOutPreview {
	_ = r.took("LogOutPreview")
	return result[core.LogOutPreview](r, "LogOutPreview", 0)
}
func (r *recorder) LogOut(_ context.Context, tellTelegram bool) error {
	return r.took("LogOut", tellTelegram)
}
func (r *recorder) EndAccount(reason core.EndReason) { _ = r.took("EndAccount", reason) }
func (r *recorder) AnswerLogin(_ context.Context, seq int, value string) error {
	return r.took("AnswerLogin", seq, value)
}
func (r *recorder) SwitchLogin(_ context.Context, seq int, to project.LoginStepKind) error {
	return r.took("SwitchLogin", seq, to)
}
func (r *recorder) SetMuted(_ context.Context, h domain.HistoryKey, muted bool) error {
	return r.took("SetMuted", h, muted)
}
func (r *recorder) SetArchived(_ context.Context, chatID int64, archived bool) error {
	return r.took("SetArchived", chatID, archived)
}
func (r *recorder) SetUnreadMark(_ context.Context, chatID int64, unread bool) error {
	return r.took("SetUnreadMark", chatID, unread)
}
func (r *recorder) AddToFolder(_ context.Context, filterID int, chatID int64, add bool) error {
	return r.took("AddToFolder", filterID, chatID, add)
}
func (r *recorder) MarkRead(_ context.Context, h domain.HistoryKey, maxID int) error {
	return r.took("MarkRead", h, maxID)
}
func (r *recorder) SetFocus(h domain.HistoryKey) { _ = r.took("SetFocus", h) }
func (r *recorder) ReadReactions(_ context.Context, h domain.HistoryKey) error {
	return r.took("ReadReactions", h)
}
func (r *recorder) ReadMentions(_ context.Context, h domain.HistoryKey) error {
	return r.took("ReadMentions", h)
}
func (r *recorder) EditMessage(_ context.Context, chatID int64, msgID int, text string, entities []domain.MessageEntity) error {
	return r.took("EditMessage", chatID, msgID, text, entities)
}
func (r *recorder) DeleteMessages(_ context.Context, chatID int64, msgIDs []int, revoke bool) error {
	return r.took("DeleteMessages", chatID, msgIDs, revoke)
}
func (r *recorder) SendReaction(_ context.Context, chatID int64, msgID int, emoji string) (bool, error) {
	err := r.took("SendReaction", chatID, msgID, emoji)
	return result[bool](r, "SendReaction", 0), err
}
func (r *recorder) Forward(_ context.Context, fromChatID int64, target domain.HistoryKey, msgIDs []int, comment string) error {
	return r.took("Forward", fromChatID, target, msgIDs, comment)
}
func (r *recorder) SetTyping(_ context.Context, h domain.HistoryKey, action domain.TypingAction) error {
	return r.took("SetTyping", h, action)
}
func (r *recorder) SaveDraft(_ context.Context, h domain.HistoryKey, text string) error {
	return r.took("SaveDraft", h, text)
}
func (r *recorder) Send(_ context.Context, req core.SendRequest) error {
	return r.took("Send", req)
}
func (r *recorder) SendMedia(_ context.Context, req core.MediaSendRequest) error {
	return r.took("SendMedia", req)
}
func (r *recorder) RetryOutbox(ref string) error   { return r.took("RetryOutbox", ref) }
func (r *recorder) DiscardOutbox(ref string) error { return r.took("DiscardOutbox", ref) }
func (r *recorder) AwaitSend(_ context.Context, ref string, within time.Duration) (core.SendOutcome, error) {
	err := r.took("AwaitSend", ref, within)
	return result[core.SendOutcome](r, "AwaitSend", 0), err
}
func (r *recorder) SearchContacts(_ context.Context, q string, limit int) ([]project.ChatRow, error) {
	err := r.took("SearchContacts", q, limit)
	return result[[]project.ChatRow](r, "SearchContacts", 0), err
}
func (r *recorder) Chats(_ context.Context) ([]project.ChatRow, error) {
	err := r.took("Chats")
	return result[[]project.ChatRow](r, "Chats", 0), err
}
func (r *recorder) Topics(_ context.Context, chatID int64) ([]project.TopicRow, error) {
	err := r.took("Topics", chatID)
	return result[[]project.TopicRow](r, "Topics", 0), err
}
func (r *recorder) Chat(_ context.Context, chatID int64) (project.ChatRow, bool, error) {
	err := r.took("Chat", chatID)
	return result[project.ChatRow](r, "Chat", 0), result[bool](r, "Chat", 1), err
}
func (r *recorder) ResolveChat(_ context.Context, name core.ChatName) (project.ChatRow, error) {
	err := r.took("ResolveChat", name)
	return result[project.ChatRow](r, "ResolveChat", 0), err
}
func (r *recorder) History(_ context.Context, h domain.HistoryKey, before, limit int) ([]domain.Message, error) {
	err := r.took("History", h, before, limit)
	return result[[]domain.Message](r, "History", 0), err
}
func (r *recorder) FolderFilters(_ context.Context) ([]domain.FolderFilter, error) {
	err := r.took("FolderFilters")
	return result[[]domain.FolderFilter](r, "FolderFilters", 0), err
}
func (r *recorder) GetParticipants(_ context.Context, chatID int64) ([]domain.ChatMember, error) {
	err := r.took("GetParticipants", chatID)
	return result[[]domain.ChatMember](r, "GetParticipants", 0), err
}
func (r *recorder) KnownUser(_ context.Context, userID int64) (domain.User, bool, error) {
	err := r.took("KnownUser", userID)
	return result[domain.User](r, "KnownUser", 0), result[bool](r, "KnownUser", 1), err
}
func (r *recorder) GetUser(_ context.Context, userID int64) (domain.User, error) {
	err := r.took("GetUser", userID)
	return result[domain.User](r, "GetUser", 0), err
}
func (r *recorder) FetchMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot) (string, error) {
	err := r.took("FetchMedia", chatID, msgID, slot)
	r.held(ctx, "FetchMedia")
	return result[string](r, "FetchMedia", 0), err
}
func (r *recorder) SaveMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot, destDir, ref string) (string, error) {
	r.mu.Lock()
	r.inBackground["SaveMedia"] = priority.IsBackground(ctx)
	r.mu.Unlock()
	err := r.took("SaveMedia", chatID, msgID, slot, destDir, ref)
	return result[string](r, "SaveMedia", 0), err
}
func (r *recorder) InvalidateMedia(chatID int64, msgID int, slot domain.MediaSlot) {
	_ = r.took("InvalidateMedia", chatID, msgID, slot)
}
func (r *recorder) FetchAvatar(_ context.Context, userID, avatarID int64) (string, error) {
	err := r.took("FetchAvatar", userID, avatarID)
	return result[string](r, "FetchAvatar", 0), err
}
func (r *recorder) InvalidateAvatar(userID, avatarID int64) {
	_ = r.took("InvalidateAvatar", userID, avatarID)
}
