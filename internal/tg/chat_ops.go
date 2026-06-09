package tg

import (
	"context"
	"fmt"
	"math"

	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func (c *GotdClient) MarkDialogUnread(ctx context.Context, peer store.Peer, unread bool) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("not connected")
	}
	return WithRetry(ctx, func() error {
		req := &tg.MessagesMarkDialogUnreadRequest{
			Peer: &tg.InputDialogPeer{Peer: peerToInput(peer)},
		}
		req.SetUnread(unread)
		_, err := api.MessagesMarkDialogUnread(ctx, req)
		return err
	})
}

func (c *GotdClient) SetMuted(ctx context.Context, peer store.Peer, muted bool) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("not connected")
	}
	return WithRetry(ctx, func() error {
		var settings tg.InputPeerNotifySettings
		if muted {
			settings.SetMuteUntil(math.MaxInt32)
		} else {
			settings.SetMuteUntil(0)
		}
		_, err := api.AccountUpdateNotifySettings(ctx, &tg.AccountUpdateNotifySettingsRequest{
			Peer:     &tg.InputNotifyPeer{Peer: peerToInput(peer)},
			Settings: settings,
		})
		return err
	})
}

// AddToFolder adds or removes peer from the include list of the dialog
// filter with the given ID. It round-trips the raw filter list from the
// server (rather than rebuilding from the lossy store model) so existing
// members keep their access hashes.
func (c *GotdClient) AddToFolder(ctx context.Context, filterID int, peer store.Peer, add bool) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("not connected")
	}
	return WithRetry(ctx, func() error {
		result, err := api.MessagesGetDialogFilters(ctx)
		if err != nil {
			return err
		}
		var target *tg.DialogFilter
		for _, f := range result.Filters {
			if df, ok := f.(*tg.DialogFilter); ok && df.ID == filterID {
				target = df
				break
			}
		}
		if target == nil {
			return fmt.Errorf("dialog filter %d not found", filterID)
		}
		input := peerToInput(peer)
		target.IncludePeers = toggleInputPeer(target.IncludePeers, input, add)
		if add {
			target.ExcludePeers = removeInputPeer(target.ExcludePeers, peer.ID)
		}
		req := &tg.MessagesUpdateDialogFilterRequest{ID: filterID}
		req.SetFilter(target)
		_, err = api.MessagesUpdateDialogFilter(ctx, req)
		return err
	})
}

func (c *GotdClient) SetArchived(ctx context.Context, peer store.Peer, archived bool) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("not connected")
	}
	folderID := 0
	if archived {
		folderID = 1
	}
	return WithRetry(ctx, func() error {
		_, err := api.FoldersEditPeerFolders(ctx, []tg.InputFolderPeer{
			{Peer: peerToInput(peer), FolderID: folderID},
		})
		return err
	})
}

// inputPeerID extracts the bare peer id from an InputPeer.
func inputPeerID(p tg.InputPeerClass) int64 {
	switch v := p.(type) {
	case *tg.InputPeerUser:
		return v.UserID
	case *tg.InputPeerChat:
		return v.ChatID
	case *tg.InputPeerChannel:
		return v.ChannelID
	}
	return 0
}

// toggleInputPeer adds p to peers (when add) or removes the entry with
// p's id (when !add), preserving order and avoiding duplicates.
func toggleInputPeer(peers []tg.InputPeerClass, p tg.InputPeerClass, add bool) []tg.InputPeerClass {
	id := inputPeerID(p)
	out := make([]tg.InputPeerClass, 0, len(peers)+1)
	found := false
	for _, e := range peers {
		if inputPeerID(e) == id {
			found = true
			if !add {
				continue
			}
		}
		out = append(out, e)
	}
	if add && !found {
		out = append(out, p)
	}
	return out
}

// removeInputPeer drops any entry with the given bare id.
func removeInputPeer(peers []tg.InputPeerClass, id int64) []tg.InputPeerClass {
	out := make([]tg.InputPeerClass, 0, len(peers))
	for _, e := range peers {
		if inputPeerID(e) != id {
			out = append(out, e)
		}
	}
	return out
}
