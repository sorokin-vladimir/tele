package ui

import (
	"context"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// Owner is the client's view of the connection owner: the subscription surface
// and nothing else. The UI holds this rather than *core.Owner so it can be
// driven by a double in tests, and so the day the owner moves behind a socket
// the client keeps the same interface.
type Owner interface {
	Subscribe(w project.Window) project.SubID
	MoveWindow(id project.SubID, w project.Window)
	Unsubscribe(id project.SubID)

	// Commands. Each applies its own optimistic change and undoes it if
	// Telegram refuses, so the client only decides how a failure looks.
	SetMuted(ctx context.Context, chatID int64, muted bool) error
	SetArchived(ctx context.Context, chatID int64, archived bool) error
	SetUnreadMark(ctx context.Context, chatID int64, unread bool) error
	AddToFolder(ctx context.Context, filterID int, chatID int64, add bool) error
	// MarkRead with maxID 0 reads the whole chat.
	MarkRead(ctx context.Context, chatID int64, maxID int) error
	// SetFocus reports which chat this client is showing, 0 for none. The owner
	// needs it because a chat you are looking at must not interrupt you; the
	// client must report leaving a chat as well as entering one, or the owner
	// goes on believing an abandoned chat is still on screen (#192).
	SetFocus(chatID int64)
	ReadReactions(ctx context.Context, chatID int64) error
	ReadMentions(ctx context.Context, chatID int64) error
	EditMessage(ctx context.Context, chatID int64, msgID int, text string, entities []domain.MessageEntity) error
	DeleteMessages(ctx context.Context, chatID int64, msgIDs []int, revoke bool) error
	SendReaction(ctx context.Context, chatID int64, msgID int, emoji string) error
	// Forward names its target by peer: it may be a search hit the owner holds
	// no chat for.
	Forward(ctx context.Context, fromChatID int64, to domain.Peer, msgIDs []int, comment string) error
	SetTyping(ctx context.Context, chatID int64, action domain.TypingAction) error
	SaveDraft(ctx context.Context, chatID int64, text string) error

	// The durable send queue. Send returns once the entry is on disk, not once
	// Telegram answered: ordering, backoff and surviving a restart are the
	// queue's business, and its entries reach the client in the projection
	// rather than being guessed at locally (#193).
	Send(ctx context.Context, req core.SendRequest) error
	// SendMedia queues local files. The client names paths and intent; the
	// upload, the album assembly and the Telegram payloads are the owner's
	// business, and none of them can cross a process boundary (#195).
	SendMedia(ctx context.Context, req core.MediaSendRequest) error
	RetryOutbox(ref string) error
	DiscardOutbox(ref string) error

	// Queries. One-off answers nobody subscribes to.
	SearchContacts(ctx context.Context, q string, limit int) ([]domain.Chat, error)
	GetParticipants(ctx context.Context, chatID int64) ([]domain.ChatMember, error)
	// KnownUser answers from what the owner already holds, without a round
	// trip, so a profile draws the moment it opens. GetUser completes it.
	//
	// A profile is asked for by id alone: the client holds no access hash for
	// the author of a message in a group, and resolving one is the owner's
	// business (ADR 0006).
	KnownUser(userID int64) (domain.User, bool)
	GetUser(ctx context.Context, userID int64) (domain.User, error)

	// Media. The owner downloads and caches; the client decodes. Paths cross
	// the boundary, never bytes (#196).
	FetchMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot) (string, error)
	SaveMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot, destDir string) (string, error)
	// InvalidateMedia drops a cached file that turned out to be undecodable, so
	// the next fetch downloads it again instead of returning the same bytes.
	InvalidateMedia(chatID int64, msgID int, slot domain.MediaSlot)
}

// refreshProjections is gone: every mutation the client makes now goes through
// an owner command, whose commit rebuilds the projections and pushes a delta.
// Nothing has to remember to repaint (#198).

// WithOwner attaches the owner the model subscribes to.
func (m RootModel) WithOwner(o Owner) RootModel {
	m.owner = o
	return m
}

// subscribeChatList opens (or re-opens) the chatlist subscription for the
// current folder and the window the list component wants.
func (m *RootModel) subscribeChatList() {
	if m.owner == nil {
		return
	}
	if m.chatListSub != 0 {
		m.owner.Unsubscribe(m.chatListSub)
	}
	offset, limit, _ := m.chatList.WindowRequest()
	m.chatListSub = m.owner.Subscribe(project.ChatListWindow{
		Folder: m.activeFolder,
		Offset: offset,
		Limit:  limit,
	})
}

// syncChatListWindow asks the owner for a wider or shifted window when the
// cursor has moved near the edge of the one the client holds. It is a no-op
// while the cursor stays inside the overscan, which is the common case.
func (m *RootModel) syncChatListWindow() {
	if m.owner == nil || m.chatListSub == 0 {
		return
	}
	offset, limit, changed := m.chatList.WindowRequest()
	if !changed {
		return
	}
	m.owner.MoveWindow(m.chatListSub, project.ChatListWindow{
		Folder: m.activeFolder,
		Offset: offset,
		Limit:  limit,
	})
}
