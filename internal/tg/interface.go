package tg

import (
	"context"
	"io"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// Client is the interface for all Telegram operations.
// All callers (ui, app) depend on this interface, not on gotd directly.
type Client interface {
	GetDialogs(ctx context.Context) ([]domain.Chat, error)
	// SearchContacts queries Telegram (contacts.search) for users matching q,
	// returning matches as domain.Chat with valid peers. Phase 1: users only.
	SearchContacts(ctx context.Context, q string, limit int) ([]domain.Chat, error)
	GetDialogFilters(ctx context.Context) ([]domain.FolderFilter, error)
	GetHistory(ctx context.Context, peer domain.Peer, offsetID int, limit int) ([]domain.Message, error)
	// RefreshMessage re-fetches a single message to obtain fresh media file
	// references (Telegram FileReferences expire).
	RefreshMessage(ctx context.Context, peer domain.Peer, msgID int) (domain.Message, error)
	// RefreshMessages re-fetches several messages in one round-trip, for the
	// media refs and grouped_id of a just-sent album.
	RefreshMessages(ctx context.Context, peer domain.Peer, ids []int) ([]domain.Message, error)
	// SendMessage sends text and returns the message it created. The message is
	// returned rather than left to the update stream because Telegram sends no
	// echo for your own message: a send into a user chat answers with an
	// updateShortSentMessage carrying an id and a date and no body, so the
	// caller has to record what it sent or nothing ever shows it (#193).
	//
	// randomID is the caller's deduplication key: Telegram deduplicates on it,
	// so it must stay the same across every retry of one logical send. That is
	// what makes an at-least-once outbox safe.
	SendMessage(ctx context.Context, peer domain.Peer, text string, replyToMsgID int, entities []domain.MessageEntity, randomID int64) (domain.Message, error)
	// GetParticipants returns mention candidates for a group/channel peer.
	GetParticipants(ctx context.Context, peer domain.Peer) ([]domain.ChatMember, error)
	// GetUser fetches a user's full profile. The address carries how the person
	// is reachable — a stored access hash, or a message they wrote — because a
	// user met only in a group has no hash anywhere on this account (#222).
	GetUser(ctx context.Context, addr UserAddress) (domain.User, error)
	// SendMedia sends a ready-made InputMediaClass via messages.sendMedia,
	// returning the confirmed message ID. It is type-agnostic: the caller builds
	// the InputMedia (photo/document/...); SendMedia knows nothing about MIME.
	SendMedia(ctx context.Context, p SendMediaParams) (int, error)
	// SendAlbum sends several already-uploaded media as one grouped album
	// (messages.sendMultiMedia), returning the message IDs in item order.
	SendAlbum(ctx context.Context, p SendAlbumParams) ([]int, error)
	// UploadFile uploads a local file in chunks and returns the resulting
	// InputFile, ready to wrap in an InputMedia. Cancel via ctx. OnProgress is
	// nil-safe and may be called concurrently-serialized by the uploader.
	UploadFile(ctx context.Context, p UploadParams) (tg.InputFileClass, error)
	// UploadMedia converts an uploaded InputFile into a server-side media ref
	// (messages.uploadMedia). Album parts require it: messages.sendMultiMedia
	// rejects raw inputMediaUploaded* constructors.
	UploadMedia(ctx context.Context, peer domain.Peer, media tg.InputMediaClass) (tg.InputMediaClass, error)
	MarkRead(ctx context.Context, peer domain.Peer, maxID int) error
	// MarkDialogUnread sets or clears the manual unread mark on a dialog.
	MarkDialogUnread(ctx context.Context, peer domain.Peer, unread bool) error
	// ReadReactions marks all unread reactions in a dialog as read
	// (messages.readReactions), clearing the unread-reaction indicator server-side.
	ReadReactions(ctx context.Context, peer domain.Peer) error
	// ReadMentions marks all unread mentions in a dialog as read
	// (messages.readMentions), clearing the unread-mention indicator server-side.
	ReadMentions(ctx context.Context, peer domain.Peer) error
	// SetMuted mutes (indefinitely) or unmutes a peer's notifications.
	SetMuted(ctx context.Context, peer domain.Peer, muted bool) error
	// AddToFolder adds or removes a peer from an existing dialog filter's
	// include list.
	AddToFolder(ctx context.Context, filterID int, peer domain.Peer, add bool) error
	// GetArchivedDialogs fetches dialogs in the built-in Archive folder
	// (folder_id 1); every returned chat has IsArchived set.
	GetArchivedDialogs(ctx context.Context) ([]domain.Chat, error)
	// SetArchived moves a peer into (archived) or out of the Archive folder.
	SetArchived(ctx context.Context, peer domain.Peer, archived bool) error
	// Downloads are streams: the owner writes them to disk and hands clients a
	// path, so nothing here returns bytes or a decoded image (#196).
	//
	// DownloadPhotoToFile streams the raw photo bytes (the size named by
	// ref.ThumbSize) into dst.
	DownloadPhotoToFile(ctx context.Context, ref domain.PhotoRef, dst io.Writer) error
	// DownloadDocumentToFile streams the full document directly into dst with
	// bounded memory, regardless of file size. It ignores ref.ThumbSize.
	DownloadDocumentToFile(ctx context.Context, ref domain.DocumentRef, dst io.Writer) error
	// DownloadDocumentThumbToFile streams the document's thumbnail
	// (ref.ThumbSize) into dst. Unlike DownloadDocumentToFile, which always
	// streams the full file, this one addresses the thumbnail location.
	DownloadDocumentThumbToFile(ctx context.Context, ref domain.DocumentRef, dst io.Writer) error
	// DownloadUserAvatarToFile streams a person's avatar into dst at the big
	// size, which is what the profile overlay draws (#236). It takes an address
	// rather than a file reference because an avatar belongs to a person rather
	// than to a message (#223).
	DownloadUserAvatarToFile(ctx context.Context, addr UserAddress, avatarID int64, dst io.Writer) error
	DeleteMessages(ctx context.Context, peer domain.Peer, ids []int, revoke bool) error
	EditMessage(ctx context.Context, peer domain.Peer, msgID int, text string, entities []domain.MessageEntity) error
	// ForwardMessages forwards messages by ID from one peer to another via
	// messages.forwardMessages. Returns ErrForwardRestricted when the source
	// chat forbids forwarding (content protection).
	ForwardMessages(ctx context.Context, from domain.Peer, to domain.Peer, ids []int) error
	SendReaction(ctx context.Context, peer domain.Peer, msgID int, emoji string) error
	SetTyping(ctx context.Context, peer domain.Peer, action domain.TypingAction) error
	// SaveDraft persists (text != "") or clears (text == "") the message draft
	// for a peer, synced with Telegram's other clients (#62).
	SaveDraft(ctx context.Context, peer domain.Peer, text string) error
	// Updates returns a channel of incoming Telegram events.
	Updates() <-chan store.Event
}
