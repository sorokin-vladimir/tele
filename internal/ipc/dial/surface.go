package dial

import (
	"context"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ipc/protocol"
)

// The surface, one call each. What each one means is said on core.Surface.

// Subscribe chooses the subscription's id here, and the host takes it. Nothing
// is waited for: the subscription's first delta is its answer.
// The subscriptions and the focus are also kept here, to be said again to a
// service reached again (#210).
func (a *Attachment) Subscribe(w project.Window) project.SubID {
	id := project.SubID(a.nextSub.Add(1))
	a.mu.Lock()
	a.subs[id] = w
	a.mu.Unlock()
	a.tell(protocol.MethodSubscribe, id, w)
	return id
}

func (a *Attachment) MoveWindow(id project.SubID, w project.Window) {
	a.mu.Lock()
	if _, ok := a.subs[id]; ok {
		a.subs[id] = w
	}
	a.mu.Unlock()
	a.tell("MoveWindow", id, w)
}

func (a *Attachment) Unsubscribe(id project.SubID) {
	a.mu.Lock()
	delete(a.subs, id)
	a.mu.Unlock()
	a.tell("Unsubscribe", id)
}

func (a *Attachment) SetFocus(h domain.HistoryKey) {
	a.mu.Lock()
	a.focus, a.focused = h, true
	a.mu.Unlock()
	a.tell("SetFocus", h)
}

func (a *Attachment) EndAccount(reason core.EndReason) { a.tell("EndAccount", reason) }
func (a *Attachment) InvalidateAvatar(userID, avatarID int64) {
	a.tell("InvalidateAvatar", userID, avatarID)
}
func (a *Attachment) RetryOutbox(ref string) error {
	return do(a, context.Background(), "RetryOutbox", ref)
}
func (a *Attachment) DiscardOutbox(ref string) error {
	return do(a, context.Background(), "DiscardOutbox", ref)
}
func (a *Attachment) AwaitSend(ctx context.Context, ref string, within time.Duration) (core.SendOutcome, error) {
	return ask[core.SendOutcome](a, ctx, "AwaitSend", ref, within)
}
func (a *Attachment) InvalidateMedia(chatID int64, msgID int, slot domain.MediaSlot) {
	a.tell("InvalidateMedia", chatID, msgID, slot)
}

// LogOutPreview answers nothing when the host cannot be asked, as it does
// between accounts.
func (a *Attachment) LogOutPreview() core.LogOutPreview {
	p, _ := ask[core.LogOutPreview](a, context.Background(), "LogOutPreview")
	return p
}

func (a *Attachment) LogOut(ctx context.Context, tellTelegram bool) error {
	return do(a, ctx, "LogOut", tellTelegram)
}

func (a *Attachment) AnswerLogin(ctx context.Context, seq int, value string) error {
	return do(a, ctx, "AnswerLogin", seq, value)
}

func (a *Attachment) SwitchLogin(ctx context.Context, seq int, to project.LoginStepKind) error {
	return do(a, ctx, "SwitchLogin", seq, to)
}

func (a *Attachment) SetMuted(ctx context.Context, h domain.HistoryKey, muted bool) error {
	return do(a, ctx, "SetMuted", h, muted)
}

func (a *Attachment) SetArchived(ctx context.Context, chatID int64, archived bool) error {
	return do(a, ctx, "SetArchived", chatID, archived)
}

func (a *Attachment) SetUnreadMark(ctx context.Context, chatID int64, unread bool) error {
	return do(a, ctx, "SetUnreadMark", chatID, unread)
}

func (a *Attachment) AddToFolder(ctx context.Context, filterID int, chatID int64, add bool) error {
	return do(a, ctx, "AddToFolder", filterID, chatID, add)
}

func (a *Attachment) MarkRead(ctx context.Context, h domain.HistoryKey, maxID int) error {
	return do(a, ctx, "MarkRead", h, maxID)
}

func (a *Attachment) ReadReactions(ctx context.Context, h domain.HistoryKey) error {
	return do(a, ctx, "ReadReactions", h)
}

func (a *Attachment) ReadMentions(ctx context.Context, h domain.HistoryKey) error {
	return do(a, ctx, "ReadMentions", h)
}

func (a *Attachment) EditMessage(ctx context.Context, chatID int64, msgID int, text string, entities []domain.MessageEntity) error {
	return do(a, ctx, "EditMessage", chatID, msgID, text, entities)
}

func (a *Attachment) DeleteMessages(ctx context.Context, chatID int64, msgIDs []int, revoke bool) error {
	return do(a, ctx, "DeleteMessages", chatID, msgIDs, revoke)
}

func (a *Attachment) SendReaction(ctx context.Context, chatID int64, msgID int, emoji string) (bool, error) {
	return ask[bool](a, ctx, "SendReaction", chatID, msgID, emoji)
}

func (a *Attachment) Forward(ctx context.Context, fromChatID int64, target domain.HistoryKey, msgIDs []int, comment string) error {
	return do(a, ctx, "Forward", fromChatID, target, msgIDs, comment)
}

func (a *Attachment) SetTyping(ctx context.Context, h domain.HistoryKey, action domain.TypingAction) error {
	return do(a, ctx, "SetTyping", h, action)
}

func (a *Attachment) SaveDraft(ctx context.Context, h domain.HistoryKey, text string) error {
	return do(a, ctx, "SaveDraft", h, text)
}

func (a *Attachment) Send(ctx context.Context, req core.SendRequest) error {
	return do(a, ctx, "Send", req)
}

func (a *Attachment) SendMedia(ctx context.Context, req core.MediaSendRequest) error {
	return do(a, ctx, "SendMedia", req)
}

func (a *Attachment) SearchContacts(ctx context.Context, q string, limit int) ([]project.ChatRow, error) {
	return ask[[]project.ChatRow](a, ctx, "SearchContacts", q, limit)
}

func (a *Attachment) Chats(ctx context.Context) ([]project.ChatRow, error) {
	return ask[[]project.ChatRow](a, ctx, "Chats")
}

func (a *Attachment) Topics(ctx context.Context, chatID int64) ([]project.TopicRow, error) {
	return ask[[]project.TopicRow](a, ctx, "Topics", chatID)
}

func (a *Attachment) Chat(ctx context.Context, chatID int64) (project.ChatRow, bool, error) {
	return ask2[project.ChatRow, bool](a, ctx, "Chat", chatID)
}

func (a *Attachment) ResolveChat(ctx context.Context, name core.ChatName) (project.ChatRow, error) {
	return ask[project.ChatRow](a, ctx, "ResolveChat", name)
}

func (a *Attachment) History(ctx context.Context, h domain.HistoryKey, before, limit int) ([]domain.Message, error) {
	return ask[[]domain.Message](a, ctx, "History", h, before, limit)
}

func (a *Attachment) FolderFilters(ctx context.Context) ([]domain.FolderFilter, error) {
	return ask[[]domain.FolderFilter](a, ctx, "FolderFilters")
}

func (a *Attachment) GetParticipants(ctx context.Context, chatID int64) ([]domain.ChatMember, error) {
	return ask[[]domain.ChatMember](a, ctx, "GetParticipants", chatID)
}

func (a *Attachment) KnownUser(ctx context.Context, userID int64) (domain.User, bool, error) {
	return ask2[domain.User, bool](a, ctx, "KnownUser", userID)
}

func (a *Attachment) GetUser(ctx context.Context, userID int64) (domain.User, error) {
	return ask[domain.User](a, ctx, "GetUser", userID)
}

func (a *Attachment) FetchMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot) (string, error) {
	return ask[string](a, ctx, "FetchMedia", chatID, msgID, slot)
}

func (a *Attachment) SaveMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot, destDir, ref string) (string, error) {
	return ask[string](a, ctx, "SaveMedia", chatID, msgID, slot, destDir, ref)
}

func (a *Attachment) FetchAvatar(ctx context.Context, userID, avatarID int64) (string, error) {
	return ask[string](a, ctx, "FetchAvatar", userID, avatarID)
}
