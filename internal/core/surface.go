package core

import (
	"context"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// Surface is everything a client may ask through its attachment: the
// subscriptions, the commands and the queries, and nothing else. The client
// holds this rather than *Attachment so it can be driven by a double in tests,
// and so that an attachment over a connection is the same thing to it as one
// in the same process (#209). It lives here rather than with the client so the
// connection's two ends can name it without depending on a client.
type Surface interface {
	Subscribe(w project.Window) project.SubID
	MoveWindow(id project.SubID, w project.Window)
	Unsubscribe(id project.SubID)

	// LogOutPreview says what logging out now would take, for the person to
	// confirm; LogOut logs out, telling Telegram first when asked to (#297).
	LogOutPreview() LogOutPreview
	LogOut(ctx context.Context, tellTelegram bool) error
	// EndAccount asks for the account to end when nothing ended its connection
	// for it, such as leaving a banned account.
	EndAccount(reason EndReason)
	// AnswerLogin answers the login step the account projection shows, named
	// by its Seq. An answer for a step already answered, here or by another
	// client, is refused (#309).
	AnswerLogin(ctx context.Context, seq int, value string) error
	// SwitchLogin leaves the login step named by seq for the other way to log
	// in: the QR step for the phone number (to LoginPhone), any other step for
	// the QR code (to LoginQR). A switch that does not fit the step, or names
	// one no longer asked, is refused (#330).
	SwitchLogin(ctx context.Context, seq int, to project.LoginStepKind) error

	// Commands. Each applies its own optimistic change and undoes it if
	// Telegram refuses, so the client only decides how a failure looks.
	SetMuted(ctx context.Context, h domain.HistoryKey, muted bool) error
	SetArchived(ctx context.Context, chatID int64, archived bool) error
	SetUnreadMark(ctx context.Context, chatID int64, unread bool) error
	AddToFolder(ctx context.Context, filterID int, chatID int64, add bool) error
	// MarkRead with maxID 0 reads the whole chat.
	MarkRead(ctx context.Context, h domain.HistoryKey, maxID int) error
	// SetFocus reports which chat this client is showing, 0 for none. The owner
	// needs it because a chat you are looking at must not interrupt you; the
	// client must report leaving a chat as well as entering one, or the owner
	// goes on believing an abandoned chat is still on screen (#192).
	SetFocus(h domain.HistoryKey)
	ReadReactions(ctx context.Context, h domain.HistoryKey) error
	ReadMentions(ctx context.Context, h domain.HistoryKey) error
	EditMessage(ctx context.Context, chatID int64, msgID int, text string, entities []domain.MessageEntity) error
	DeleteMessages(ctx context.Context, chatID int64, msgIDs []int, revoke bool) error
	// SendReaction answers kept=false when Telegram accepted the request but
	// did not keep the reaction; the set it did keep is already on its way.
	SendReaction(ctx context.Context, chatID int64, msgID int, emoji string) (kept bool, err error)
	Forward(ctx context.Context, fromChatID int64, target domain.HistoryKey, msgIDs []int, comment string) error
	SetTyping(ctx context.Context, h domain.HistoryKey, action domain.TypingAction) error
	SaveDraft(ctx context.Context, h domain.HistoryKey, text string) error

	// The durable send queue. Send returns once the entry is on disk, not once
	// Telegram answered: ordering, backoff and surviving a restart are the
	// queue's business, and its entries reach the client in the projection
	// rather than being guessed at locally (#193).
	Send(ctx context.Context, req SendRequest) error
	// SendMedia queues local files. The client names paths and intent; the
	// upload, the album assembly and the Telegram payloads are the owner's
	// business, and none of them can cross a process boundary (#195).
	SendMedia(ctx context.Context, req MediaSendRequest) error
	RetryOutbox(ref string) error
	DiscardOutbox(ref string) error
	// AwaitSend waits up to within for a send to become a message or fail,
	// and answers with where it stands then; a command waits on its send this
	// way, a window watches the projection (#181).
	AwaitSend(ctx context.Context, ref string, within time.Duration) (SendOutcome, error)

	// Queries. One-off answers nobody subscribes to. They take a context and
	// may fail because in v2 each is a round trip to another process, and the
	// client asks them from a command rather than from Update (#278).
	SearchContacts(ctx context.Context, q string, limit int) ([]project.ChatRow, error)
	// Chats is every chat the owner holds, archived ones included, in list
	// order: what search and the forward picker look through.
	Chats(ctx context.Context) ([]project.ChatRow, error)
	// Topics answers with a forum's topics, for a modal choosing one (#275).
	Topics(ctx context.Context, chatID int64) ([]project.TopicRow, error)
	// Chat is one chat the owner holds; false for a person it can only address.
	Chat(ctx context.Context, chatID int64) (project.ChatRow, bool, error)
	// ResolveChat answers with the chat a command named, by id, username or
	// as me; one nobody holds is PeerNotFound (#181).
	ResolveChat(ctx context.Context, name ChatName) (project.ChatRow, error)
	// History answers with up to limit messages of a history older than
	// before, the newest for zero, oldest first, fetching what the store
	// lacks. It is how a command reads; a window subscribes (#181).
	History(ctx context.Context, h domain.HistoryKey, before, limit int) ([]domain.Message, error)
	// HistoryAround answers with up to limit messages of a history around a
	// message, oldest first, from Telegram and kept nowhere; a message that is
	// not there is NotFound (#348).
	HistoryAround(ctx context.Context, h domain.HistoryKey, id, limit int) ([]domain.Message, error)
	// SearchMessages asks Telegram which messages hold a text within a scope,
	// newest first (#348), one page from after down, and where the next page
	// starts (#349).
	SearchMessages(ctx context.Context, scope project.SearchScope, text string, after project.SearchPosition, limit int) (project.SearchPage, error)
	FolderFilters(ctx context.Context) ([]domain.FolderFilter, error)
	GetParticipants(ctx context.Context, chatID int64) ([]domain.ChatMember, error)
	// KnownUser answers from what the owner already holds, without asking
	// Telegram, and a profile opens on it. GetUser completes it.
	//
	// A profile is asked for by id alone: the client holds no access hash for
	// the author of a message in a group, and resolving one is the owner's
	// business (ADR 0006).
	KnownUser(ctx context.Context, userID int64) (domain.User, bool, error)
	GetUser(ctx context.Context, userID int64) (domain.User, error)

	// Media. The owner downloads and caches; the client decodes. Paths cross
	// the boundary, never bytes (#196).
	FetchMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot) (string, error)
	SaveMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot, destDir, ref string) (string, error)
	// InvalidateMedia drops a cached file that turned out to be undecodable, so
	// the next fetch downloads it again instead of returning the same bytes.
	InvalidateMedia(chatID int64, msgID int, slot domain.MediaSlot)

	// FetchAvatar downloads a person's avatar, cached apart from chat media
	// (#223, ADR 0007). avatarID is the one the owner handed over in
	// domain.User: passing it back is what makes a changed picture a different
	// file, so the client never has to notice the change itself.
	FetchAvatar(ctx context.Context, userID, avatarID int64) (string, error)
	InvalidateAvatar(userID, avatarID int64)
}

// The in-process attachment offers the whole surface; asserted here so the
// two cannot drift apart unnoticed (#309).
var _ Surface = (*Attachment)(nil)
