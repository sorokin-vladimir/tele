package tg

import (
	"context"
	"image"
	"io"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// Client is the interface for all Telegram operations.
// All callers (ui, app) depend on this interface, not on gotd directly.
type Client interface {
	GetDialogs(ctx context.Context) ([]store.Chat, error)
	GetDialogFilters(ctx context.Context) ([]store.FolderFilter, error)
	GetHistory(ctx context.Context, peer store.Peer, offsetID int, limit int) ([]store.Message, error)
	// RefreshMessage re-fetches a single message to obtain fresh media file
	// references (Telegram FileReferences expire).
	RefreshMessage(ctx context.Context, peer store.Peer, msgID int) (store.Message, error)
	SendMessage(ctx context.Context, peer store.Peer, text string, replyToMsgID int) (int, error)
	// SendMedia sends a ready-made InputMediaClass via messages.sendMedia,
	// returning the confirmed message ID. It is type-agnostic: the caller builds
	// the InputMedia (photo/document/...); SendMedia knows nothing about MIME.
	SendMedia(ctx context.Context, p SendMediaParams) (int, error)
	// UploadFile uploads a local file in chunks and returns the resulting
	// InputFile, ready to wrap in an InputMedia. Cancel via ctx. OnProgress is
	// nil-safe and may be called concurrently-serialized by the uploader.
	UploadFile(ctx context.Context, p UploadParams) (tg.InputFileClass, error)
	MarkRead(ctx context.Context, peer store.Peer, maxID int) error
	// MarkDialogUnread sets or clears the manual unread mark on a dialog.
	MarkDialogUnread(ctx context.Context, peer store.Peer, unread bool) error
	// SetMuted mutes (indefinitely) or unmutes a peer's notifications.
	SetMuted(ctx context.Context, peer store.Peer, muted bool) error
	// AddToFolder adds or removes a peer from an existing dialog filter's
	// include list.
	AddToFolder(ctx context.Context, filterID int, peer store.Peer, add bool) error
	// GetArchivedDialogs fetches dialogs in the built-in Archive folder
	// (folder_id 1); every returned chat has IsArchived set.
	GetArchivedDialogs(ctx context.Context) ([]store.Chat, error)
	// SetArchived moves a peer into (archived) or out of the Archive folder.
	SetArchived(ctx context.Context, peer store.Peer, archived bool) error
	DownloadPhoto(ctx context.Context, ref store.PhotoRef) (image.Image, error)
	// DownloadDocument fetches the full document file as raw bytes. Suitable
	// only for small documents (e.g. voice messages); large files should use
	// DownloadDocumentToFile to avoid buffering in memory.
	DownloadDocument(ctx context.Context, ref store.DocumentRef) ([]byte, error)
	// DownloadDocumentToFile streams the full document directly into dst with
	// bounded memory, regardless of file size.
	DownloadDocumentToFile(ctx context.Context, ref store.DocumentRef, dst io.Writer) error
	// DownloadDocumentThumb fetches and decodes the document's thumbnail
	// (ref.ThumbSize) for an inline preview.
	DownloadDocumentThumb(ctx context.Context, ref store.DocumentRef) (image.Image, error)
	// DownloadDocumentImage fetches the full document file and decodes it as an
	// image (used for static WEBP stickers; streams the main file, not a thumb).
	DownloadDocumentImage(ctx context.Context, ref store.DocumentRef) (image.Image, error)
	DeleteMessages(ctx context.Context, peer store.Peer, ids []int, revoke bool) error
	EditMessage(ctx context.Context, peer store.Peer, msgID int, text string) error
	SendReaction(ctx context.Context, peer store.Peer, msgID int, emoji string) error
	SetTyping(ctx context.Context, peer store.Peer, action store.TypingAction) error
	// SaveDraft persists (text != "") or clears (text == "") the message draft
	// for a peer, synced with Telegram's other clients (#62).
	SaveDraft(ctx context.Context, peer store.Peer, text string) error
	// Updates returns a channel of incoming Telegram events.
	Updates() <-chan store.Event
}
