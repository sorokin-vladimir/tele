package tg

import (
	"context"
	"image"

	"github.com/sorokin-vladimir/tele/internal/store"
)

// Client is the interface for all Telegram operations.
// All callers (ui, app) depend on this interface, not on gotd directly.
type Client interface {
	GetDialogs(ctx context.Context) ([]store.Chat, error)
	GetDialogFilters(ctx context.Context) ([]store.FolderFilter, error)
	GetHistory(ctx context.Context, peer store.Peer, offsetID int, limit int) ([]store.Message, error)
	SendMessage(ctx context.Context, peer store.Peer, text string, replyToMsgID int) (int, error)
	MarkRead(ctx context.Context, peer store.Peer, maxID int) error
	DownloadPhoto(ctx context.Context, ref store.PhotoRef) (image.Image, error)
	DeleteMessages(ctx context.Context, peer store.Peer, ids []int, revoke bool) error
	EditMessage(ctx context.Context, peer store.Peer, msgID int, text string) error
	SendReaction(ctx context.Context, peer store.Peer, msgID int, emoji string) error
	SetTyping(ctx context.Context, peer store.Peer, action store.TypingAction) error
	// Updates returns a channel of incoming Telegram events.
	Updates() <-chan store.Event
}
