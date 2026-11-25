package core

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// Attachment is a client's one link to a host, for as long as the client wants
// it. It outlives owners: the host seats it at each owner in turn, and the
// subscriptions made through it belong to the owner of the moment and end with
// that owner, while the attachment carries on to the next one. It carries the
// whole owner command surface plus what is the client's own: its subscription
// ids, the one stream everything addressed to it arrives on, and the requests
// it makes of the host (#309).
//
// Over a socket an attachment is a connection; here it is a handle, and the
// two behave alike.
type Attachment struct {
	// stream carries deltas and events to the client, in the order they were
	// produced, from every owner the attachment has been seated at.
	stream chan any
	// nextSub is the last subscription id handed out. Ids only grow, across
	// owners too, so a delta of a subscription just replaced is never read as
	// the new one's.
	nextSub atomic.Uint64
	// endRequests carries the client's request that the account end.
	endRequests chan EndReason
	// done is closed when the client detaches, so the host never waits on a
	// client that is gone.
	done     chan struct{}
	doneOnce sync.Once

	mu sync.Mutex
	// seat is the attachment's place at the owner of the moment; nil between
	// owners.
	seat *seat
	// calls ends when the owner of the moment does, and every call made
	// through the attachment ends with it.
	calls    context.Context
	endCalls context.CancelFunc
	// pending holds the subscriptions made while no owner was seated, which
	// the next one is given. A client's next screen subscribes as soon as the
	// last account ended, before the next one is open.
	pending map[project.SubID]project.Window
}

// NewAttachment makes an attachment that is not seated at any owner yet.
func NewAttachment() *Attachment {
	return &Attachment{
		stream:      make(chan any, StreamSize),
		endRequests: make(chan EndReason, 1),
		done:        make(chan struct{}),
		pending:     make(map[project.SubID]project.Window),
	}
}

// Stream is everything addressed to this client, deltas and events alike, in
// the order they were produced: project.Delta, Incoming, Typing, Progress,
// DownloadProgress, Failure and Notification from the owner, and AccountEnded,
// HostFailure and HostStopping from the host.
func (a *Attachment) Stream() <-chan any { return a.stream }

// Join seats the attachment at o, the owner now current, and makes there the
// subscriptions the client made while there was none.
func (a *Attachment) Join(o *Owner) {
	s := o.attach(a.stream)
	a.mu.Lock()
	a.seat = s
	a.calls, a.endCalls = context.WithCancel(context.Background())
	pending := a.pending
	a.pending = make(map[project.SubID]project.Window)
	a.mu.Unlock()
	// In the order they were made: a later one may replace an earlier one's
	// window, and ids only grow.
	for _, id := range slices.Sorted(maps.Keys(pending)) {
		s.subscribe(id, pending[id])
	}
}

// Leave unseats the attachment from the owner that ended, and tells the client
// so. Nothing that owner says reaches the client after this; its
// subscriptions are gone, and every call still in flight through it is
// answered with AccountEnded.
func (a *Attachment) Leave(ended AccountEnded) {
	a.unseat()
	a.deliver(ended)
}

// Detach ends the attachment: the client is gone. Its seat is given up, and
// the host stops waiting on it.
func (a *Attachment) Detach() {
	a.unseat()
	a.doneOnce.Do(func() { close(a.done) })
}

func (a *Attachment) unseat() {
	a.mu.Lock()
	s := a.seat
	a.seat = nil
	if a.endCalls != nil {
		a.endCalls()
	}
	a.mu.Unlock()
	if s != nil {
		s.leave()
	}
}

// HostFailure is something the host itself could not do, said to the client
// because nobody else is there to say it to: an account that ended and could
// not be removed, a next one that could not be opened. Err is a domain error,
// since it is a value that crosses to a client.
type HostFailure struct {
	Err *telerr.Error
}

// HostStopping tells a client that the host is stopping: the tele process
// that held the account is quitting, and the connection ends after this. It
// is what tells a host that quit from one that broke (#208), and Reason is
// why it quits.
type HostStopping struct {
	Reason StopReason
}

// StopReason is why a host stops.
type StopReason string

const (
	// StopQuit: the host was told to quit, by the person or by whatever keeps
	// it running.
	StopQuit StopReason = "quit"
	// StopRestart: a newer client asked the service to restart, and its service
	// manager starts it again on the binary installed now (#183).
	StopRestart StopReason = "restart"
	// StopUninstalled: the service was uninstalled, and nothing starts it
	// again. Its clients do not wait for it (#182).
	StopUninstalled StopReason = "uninstalled"
)

// Tell tells the client something from the host. Unlike the owner's events it
// waits for the client to take it, since nothing repeats it, and gives up only
// if the client goes.
func (a *Attachment) Tell(ev any) { a.deliver(ev) }

func (a *Attachment) deliver(ev any) {
	select {
	case a.stream <- ev:
	case <-a.done:
	}
}

// EndAccount asks the host to end the account for reason. It is for an end the
// connection did not bring about itself: a banned account's connection is
// already over, and the person leaves it to log in with another number. One
// request waiting is enough; more are dropped.
func (a *Attachment) EndAccount(reason EndReason) {
	select {
	case a.endRequests <- reason:
	default:
	}
}

// EndRequests is where the host hears that the client asked for the account to
// end.
func (a *Attachment) EndRequests() <-chan EndReason { return a.endRequests }

// current is the seat now, or nil between owners.
func (a *Attachment) current() *seat {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.seat
}

func accountEnded() error { return &telerr.Error{Kind: telerr.AccountEnded} }

// call runs fn against the owner of the moment, under a context that also ends
// when that owner does. A call made between owners, or one whose owner ended
// while it ran, is answered with AccountEnded whatever fn said: the account it
// was meant for is gone, and its answer is nobody's to act on.
func call[T any](a *Attachment, ctx context.Context, fn func(context.Context, *seat) (T, error)) (T, error) {
	var zero T
	a.mu.Lock()
	s, calls := a.seat, a.calls
	a.mu.Unlock()
	if s == nil {
		return zero, accountEnded()
	}
	cctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(calls, cancel)
	defer func() {
		stop()
		cancel()
	}()
	v, err := fn(cctx, s)
	// calls ends before the seat is marked left, so a call cut off by the end
	// can return in between: either one means the owner is gone.
	if s.hasLeft() || calls.Err() != nil {
		return zero, accountEnded()
	}
	return v, err
}

// do is call for a command that answers with nothing but an error.
func do(a *Attachment, ctx context.Context, fn func(context.Context, *seat) error) error {
	_, err := call(a, ctx, func(ctx context.Context, s *seat) (struct{}, error) {
		return struct{}{}, fn(ctx, s)
	})
	return err
}

// Subscribe registers a window under the next id of this client's and returns
// it. It does not wait for the owner: the subscription's first delta is its
// answer. Over a connection the client side chooses the id the same way, and
// SubscribeAs takes it. Made between owners, it waits for the next one.
func (a *Attachment) Subscribe(w project.Window) project.SubID {
	id := project.SubID(a.nextSub.Add(1))
	a.subscribe(id, w)
	return id
}

// SubscribeAs registers a window under an id the client chose: over a
// connection the client side hands the ids out, and this side holds it to
// them. An id must be past every one before it, or a delta of a subscription
// already replaced could be read as the new one's.
func (a *Attachment) SubscribeAs(id project.SubID, w project.Window) error {
	for {
		last := a.nextSub.Load()
		if uint64(id) <= last {
			return fmt.Errorf("subscription %d is not past %d", id, last)
		}
		if a.nextSub.CompareAndSwap(last, uint64(id)) {
			break
		}
	}
	a.subscribe(id, w)
	return nil
}

func (a *Attachment) subscribe(id project.SubID, w project.Window) {
	a.mu.Lock()
	s := a.seat
	if s == nil {
		a.pending[id] = w
	}
	a.mu.Unlock()
	if s != nil {
		s.subscribe(id, w)
	}
}

// MoveWindow repositions a subscription.
func (a *Attachment) MoveWindow(id project.SubID, w project.Window) {
	a.mu.Lock()
	s := a.seat
	if _, waiting := a.pending[id]; waiting {
		a.pending[id] = w
	}
	a.mu.Unlock()
	if s != nil {
		s.moveWindow(id, w)
	}
}

// Unsubscribe drops a subscription.
func (a *Attachment) Unsubscribe(id project.SubID) {
	a.mu.Lock()
	s := a.seat
	delete(a.pending, id)
	a.mu.Unlock()
	if s != nil {
		s.registry.Unsubscribe(id)
		s.syncShown()
	}
}

// SetFocus reports the history this client is showing - a chat, or one topic
// of a forum - and the zero key for none. Between owners there is nothing to
// tell: nothing can notify.
func (a *Attachment) SetFocus(h domain.HistoryKey) {
	if s := a.current(); s != nil {
		s.setFocus(h)
	}
}

// LogOutPreview says what logging out now would take; nothing between owners.
func (a *Attachment) LogOutPreview() LogOutPreview {
	if s := a.current(); s != nil {
		return s.LogOutPreview()
	}
	return LogOutPreview{}
}

func (a *Attachment) LogOut(ctx context.Context, tellTelegram bool) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.LogOut(ctx, tellTelegram) })
}

func (a *Attachment) AnswerLogin(ctx context.Context, seq int, value string) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.AnswerLogin(ctx, seq, value) })
}

func (a *Attachment) SwitchLogin(ctx context.Context, seq int, to project.LoginStepKind) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.SwitchLogin(ctx, seq, to) })
}

func (a *Attachment) SetMuted(ctx context.Context, h domain.HistoryKey, muted bool) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.SetMuted(ctx, h, muted) })
}

func (a *Attachment) SetArchived(ctx context.Context, chatID int64, archived bool) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.SetArchived(ctx, chatID, archived) })
}

func (a *Attachment) SetUnreadMark(ctx context.Context, chatID int64, unread bool) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.SetUnreadMark(ctx, chatID, unread) })
}

func (a *Attachment) AddToFolder(ctx context.Context, filterID int, chatID int64, add bool) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.AddToFolder(ctx, filterID, chatID, add) })
}

func (a *Attachment) MarkRead(ctx context.Context, h domain.HistoryKey, maxID int) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.MarkRead(ctx, h, maxID) })
}

func (a *Attachment) ReadReactions(ctx context.Context, h domain.HistoryKey) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.ReadReactions(ctx, h) })
}

func (a *Attachment) ReadMentions(ctx context.Context, h domain.HistoryKey) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.ReadMentions(ctx, h) })
}

func (a *Attachment) EditMessage(ctx context.Context, chatID int64, msgID int, text string, entities []domain.MessageEntity) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.EditMessage(ctx, chatID, msgID, text, entities) })
}

func (a *Attachment) DeleteMessages(ctx context.Context, chatID int64, msgIDs []int, revoke bool) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.DeleteMessages(ctx, chatID, msgIDs, revoke) })
}

func (a *Attachment) SendReaction(ctx context.Context, chatID int64, msgID int, emoji string) (bool, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) (bool, error) { return s.SendReaction(ctx, chatID, msgID, emoji) })
}

func (a *Attachment) Forward(ctx context.Context, fromChatID int64, target domain.HistoryKey, msgIDs []int, comment string) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.Forward(ctx, fromChatID, target, msgIDs, comment) })
}

func (a *Attachment) SetTyping(ctx context.Context, h domain.HistoryKey, action domain.TypingAction) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.SetTyping(ctx, h, action) })
}

func (a *Attachment) SaveDraft(ctx context.Context, h domain.HistoryKey, text string) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.SaveDraft(ctx, h, text) })
}

func (a *Attachment) Send(ctx context.Context, req SendRequest) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.Send(ctx, req) })
}

func (a *Attachment) SendMedia(ctx context.Context, req MediaSendRequest) error {
	return do(a, ctx, func(ctx context.Context, s *seat) error { return s.SendMedia(ctx, req) })
}

func (a *Attachment) RetryOutbox(ref string) error {
	return do(a, context.Background(), func(_ context.Context, s *seat) error { return s.RetryOutbox(ref) })
}

func (a *Attachment) DiscardOutbox(ref string) error {
	return do(a, context.Background(), func(_ context.Context, s *seat) error { return s.DiscardOutbox(ref) })
}

func (a *Attachment) AwaitSend(ctx context.Context, ref string, within time.Duration) (SendOutcome, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) (SendOutcome, error) { return s.AwaitSend(ctx, ref, within) })
}

func (a *Attachment) SearchContacts(ctx context.Context, q string, limit int) ([]project.ChatRow, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) ([]project.ChatRow, error) { return s.SearchContacts(ctx, q, limit) })
}

func (a *Attachment) Chats(ctx context.Context) ([]project.ChatRow, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) ([]project.ChatRow, error) { return s.Chats(ctx) })
}

func (a *Attachment) Topics(ctx context.Context, chatID int64) ([]project.TopicRow, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) ([]project.TopicRow, error) { return s.Topics(ctx, chatID) })
}

func (a *Attachment) Chat(ctx context.Context, chatID int64) (project.ChatRow, bool, error) {
	type found struct {
		row project.ChatRow
		ok  bool
	}
	f, err := call(a, ctx, func(ctx context.Context, s *seat) (found, error) {
		row, ok, err := s.Chat(ctx, chatID)
		return found{row, ok}, err
	})
	return f.row, f.ok, err
}

func (a *Attachment) ResolveChat(ctx context.Context, name ChatName) (project.ChatRow, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) (project.ChatRow, error) { return s.ResolveChat(ctx, name) })
}

func (a *Attachment) History(ctx context.Context, h domain.HistoryKey, before, limit int) ([]domain.Message, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) ([]domain.Message, error) { return s.History(ctx, h, before, limit) })
}

func (a *Attachment) HistoryAround(ctx context.Context, h domain.HistoryKey, id, limit int) ([]domain.Message, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) ([]domain.Message, error) {
		return s.HistoryAround(ctx, h, id, limit)
	})
}

func (a *Attachment) SearchMessages(ctx context.Context, scope project.SearchScope, text string, after project.SearchPosition, limit int) (project.SearchPage, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) (project.SearchPage, error) {
		return s.SearchMessages(ctx, scope, text, after, limit)
	})
}

func (a *Attachment) FolderFilters(ctx context.Context) ([]domain.FolderFilter, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) ([]domain.FolderFilter, error) { return s.FolderFilters(ctx) })
}

func (a *Attachment) GetParticipants(ctx context.Context, chatID int64) ([]domain.ChatMember, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) ([]domain.ChatMember, error) { return s.GetParticipants(ctx, chatID) })
}

func (a *Attachment) KnownUser(ctx context.Context, userID int64) (domain.User, bool, error) {
	type found struct {
		user domain.User
		ok   bool
	}
	f, err := call(a, ctx, func(ctx context.Context, s *seat) (found, error) {
		u, ok, err := s.KnownUser(ctx, userID)
		return found{u, ok}, err
	})
	return f.user, f.ok, err
}

func (a *Attachment) GetUser(ctx context.Context, userID int64) (domain.User, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) (domain.User, error) { return s.GetUser(ctx, userID) })
}

func (a *Attachment) FetchMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot) (string, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) (string, error) { return s.FetchMedia(ctx, chatID, msgID, slot) })
}

// SaveMedia streams the named media into destDir, bypassing the cache, and
// returns the path it actually wrote. The owner picks the name: it follows from
// the document's own name or its MIME type, which is domain knowledge rather
// than rendering.
//
// A non-empty ref asks for progress: it comes back on this client's stream,
// under that ref, each time the whole percentage changes, and to nobody else -
// the course of a call is the caller's to watch. Media of no known size reports
// nothing, since there is no percentage to give (#204).
func (a *Attachment) SaveMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot, destDir, ref string) (string, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) (string, error) {
		return s.saveMedia(ctx, chatID, msgID, slot, destDir, ref)
	})
}

func (a *Attachment) InvalidateMedia(chatID int64, msgID int, slot domain.MediaSlot) {
	if s := a.current(); s != nil {
		s.InvalidateMedia(chatID, msgID, slot)
	}
}

func (a *Attachment) FetchAvatar(ctx context.Context, userID, avatarID int64) (string, error) {
	return call(a, ctx, func(ctx context.Context, s *seat) (string, error) { return s.FetchAvatar(ctx, userID, avatarID) })
}

func (a *Attachment) InvalidateAvatar(userID, avatarID int64) {
	if s := a.current(); s != nil {
		s.InvalidateAvatar(userID, avatarID)
	}
}
