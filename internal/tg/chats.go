package tg

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func (c *GotdClient) GetDialogs(ctx context.Context) ([]store.Chat, error) {
	return c.getDialogs(ctx, 0)
}

func (c *GotdClient) GetArchivedDialogs(ctx context.Context) ([]store.Chat, error) {
	return c.getDialogs(ctx, 1)
}

func (c *GotdClient) getDialogs(ctx context.Context, folderID int) ([]store.Chat, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}

	c.log.Debug("GetDialogs start")

	const pageSize = 100
	const maxPages = 30 // safety cap: 3000 dialogs max

	// Accumulators across pages (deduplicate by peer ID)
	seenPeer := make(map[int64]struct{})
	var allDialogs []tg.DialogClass
	var allMsgs []tg.MessageClass
	userMap := make(map[int64]*tg.User)
	chatMap := make(map[int64]tg.ChatClass)

	offsetDate, offsetID := 0, 0
	var offsetPeer tg.InputPeerClass = &tg.InputPeerEmpty{}

	for page := 0; page < maxPages; page++ {
		var result tg.MessagesDialogsClass
		err := WithRetry(ctx, func() error {
			var err error
			req := &tg.MessagesGetDialogsRequest{
				Limit:      pageSize,
				OffsetDate: offsetDate,
				OffsetID:   offsetID,
				OffsetPeer: offsetPeer,
			}
			if folderID != 0 {
				req.SetFolderID(folderID)
			}
			result, err = api.MessagesGetDialogs(ctx, req)
			if err != nil {
				c.log.Error("MessagesGetDialogs failed", zap.Error(err))
			}
			return err
		})
		if err != nil {
			return nil, err
		}

		var dialogs []tg.DialogClass
		var msgs []tg.MessageClass
		var users []tg.UserClass
		var chatsRaw []tg.ChatClass
		totalExpected := 0
		done := false

		switch v := result.(type) {
		case *tg.MessagesDialogs:
			dialogs, msgs, users, chatsRaw = v.Dialogs, v.Messages, v.Users, v.Chats
			done = true
		case *tg.MessagesDialogsSlice:
			dialogs, msgs, users, chatsRaw = v.Dialogs, v.Messages, v.Users, v.Chats
			totalExpected = v.Count
		default:
			done = true
		}

		// Merge users and chats into maps (latest wins on collision)
		for _, u := range users {
			if user, ok := u.(*tg.User); ok {
				userMap[user.ID] = user
			}
		}
		for _, ch := range chatsRaw {
			switch v := ch.(type) {
			case *tg.Chat:
				chatMap[v.ID] = v
			case *tg.Channel:
				chatMap[v.ID] = v
			}
		}

		// Append new (non-duplicate) dialogs
		newCount := 0
		for _, d := range dialogs {
			dlg, ok := d.(*tg.Dialog)
			if !ok {
				continue
			}
			id := peerIDFromPeer(dlg.Peer)
			if id == 0 {
				continue
			}
			if _, dup := seenPeer[id]; dup {
				continue
			}
			seenPeer[id] = struct{}{}
			allDialogs = append(allDialogs, d)
			newCount++
		}
		allMsgs = append(allMsgs, msgs...)

		c.log.Debug("GetDialogs page",
			zap.Int("page", page),
			zap.Int("new", newCount),
			zap.Int("total", len(allDialogs)),
			zap.Int("expected", totalExpected),
		)

		if done || newCount == 0 {
			break
		}
		if totalExpected > 0 && len(allDialogs) >= totalExpected {
			break
		}

		// Compute next page offset from last dialog in this batch
		var lastDlg *tg.Dialog
		for i := len(dialogs) - 1; i >= 0; i-- {
			if dlg, ok := dialogs[i].(*tg.Dialog); ok {
				lastDlg = dlg
				break
			}
		}
		if lastDlg == nil {
			break
		}

		// Find the top_message date for this dialog
		msgDateIndex := make(map[int]int, len(msgs))
		for _, raw := range msgs {
			if m, ok := raw.(*tg.Message); ok {
				msgDateIndex[m.ID] = m.Date
			}
		}
		nextDate := msgDateIndex[lastDlg.TopMessage]
		if nextDate == 0 {
			break
		}
		offsetDate = nextDate
		offsetID = lastDlg.TopMessage
		offsetPeer = buildOffsetPeer(lastDlg.Peer, userMap, chatMap)
	}

	// Build final result using the accumulated data
	userSlice := make([]tg.UserClass, 0, len(userMap))
	for _, u := range userMap {
		userSlice = append(userSlice, u)
	}
	chatSlice := make([]tg.ChatClass, 0, len(chatMap))
	for _, ch := range chatMap {
		chatSlice = append(chatSlice, ch)
	}
	synthetic := &tg.MessagesDialogs{
		Dialogs:  allDialogs,
		Messages: allMsgs,
		Users:    userSlice,
		Chats:    chatSlice,
	}
	chats := c.parseDialogs(synthetic)
	c.log.Debug("GetDialogs done", zap.Int("count", len(chats)))
	return chats, nil
}

// buildOffsetPeer constructs the InputPeer for getDialogs pagination offset.
func buildOffsetPeer(peer tg.PeerClass, userMap map[int64]*tg.User, chatMap map[int64]tg.ChatClass) tg.InputPeerClass {
	switch p := peer.(type) {
	case *tg.PeerUser:
		if u, ok := userMap[p.UserID]; ok {
			return &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}
		}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: p.ChatID}
	case *tg.PeerChannel:
		if ch, ok := chatMap[p.ChannelID].(*tg.Channel); ok {
			return &tg.InputPeerChannel{ChannelID: ch.ID, AccessHash: ch.AccessHash}
		}
	}
	return &tg.InputPeerEmpty{}
}

type dialogMeta struct {
	pinned    bool
	lastMsgAt time.Time
}

func (c *GotdClient) parseDialogs(result tg.MessagesDialogsClass) []store.Chat {
	var dialogs []tg.DialogClass
	var msgs []tg.MessageClass
	var users []tg.UserClass
	var chatsRaw []tg.ChatClass

	switch v := result.(type) {
	case *tg.MessagesDialogs:
		dialogs, msgs, users, chatsRaw = v.Dialogs, v.Messages, v.Users, v.Chats
	case *tg.MessagesDialogsSlice:
		dialogs, msgs, users, chatsRaw = v.Dialogs, v.Messages, v.Users, v.Chats
	default:
		return nil
	}

	// Build message date index: msgID → date
	msgDate := make(map[int]time.Time, len(msgs))
	for _, raw := range msgs {
		if m, ok := raw.(*tg.Message); ok {
			msgDate[m.ID] = time.Unix(int64(m.Date), 0)
		}
	}

	// Build dialog meta index: peerID → meta
	meta := make(map[int64]dialogMeta, len(dialogs))
	for _, d := range dialogs {
		dlg, ok := d.(*tg.Dialog)
		if !ok {
			continue
		}
		id := peerIDFromPeer(dlg.Peer)
		if id == 0 {
			continue
		}
		meta[id] = dialogMeta{
			pinned:    dlg.Pinned,
			lastMsgAt: msgDate[dlg.TopMessage],
		}
	}

	// Build peer lookup maps
	userMap := make(map[int64]*tg.User, len(users))
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}
	chatMap := make(map[int64]tg.ChatClass, len(chatsRaw))
	for _, ch := range chatsRaw {
		switch v := ch.(type) {
		case *tg.Chat:
			chatMap[v.ID] = v
		case *tg.Channel:
			chatMap[v.ID] = v
		}
	}

	// Preserve dialog order from server, apply meta
	out := make([]store.Chat, 0, len(dialogs))
	for _, d := range dialogs {
		dlg, ok := d.(*tg.Dialog)
		if !ok {
			continue
		}
		id := peerIDFromPeer(dlg.Peer)
		if id == 0 {
			continue
		}
		m := meta[id]
		var chat store.Chat
		var converted bool
		switch dlg.Peer.(type) {
		case *tg.PeerUser:
			if u, ok := userMap[id]; ok {
				chat, converted = convertUser(u)
			}
		case *tg.PeerChat:
			if ch, ok := chatMap[id].(*tg.Chat); ok {
				chat, converted = convertGroupChat(ch)
			}
		case *tg.PeerChannel:
			if ch, ok := chatMap[id].(*tg.Channel); ok {
				chat, converted = convertChannel(ch)
			}
		}
		if !converted {
			continue
		}
		chat.IsMuted = isMuted(dlg)
		// Derive archived state from the dialog's own folder, not from which
		// folder we queried: messages.getDialogs(folder_id=1) also returns the
		// main folder's pinned dialogs, which must not be marked archived.
		folderID, _ := dlg.GetFolderID()
		chat.IsArchived = folderID == 1
		chat.Pinned = m.pinned
		chat.UnreadCount = dlg.UnreadCount
		chat.ReadInboxMaxID = dlg.ReadInboxMaxID
		chat.ReadOutboxMaxID = dlg.ReadOutboxMaxID
		chat.LastMessage = &store.Message{Date: m.lastMsgAt}
		if d, ok := dlg.GetDraft(); ok {
			chat.Draft = draftText(d)
		}
		c.cachePeer(chat.Peer)
		out = append(out, chat)
	}

	// Sort: pinned first (preserving server order within group), then by last message date desc
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].LastMessage.Date.After(out[j].LastMessage.Date)
	})

	return out
}

// draftText extracts the plain draft text from a DraftMessageClass. A
// DraftMessageEmpty (or any non-text draft) yields "".
func draftText(d tg.DraftMessageClass) string {
	if dm, ok := d.(*tg.DraftMessage); ok {
		return dm.Message
	}
	return ""
}

func peerIDFromPeer(peer tg.PeerClass) int64 {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return p.UserID
	case *tg.PeerChat:
		return p.ChatID
	case *tg.PeerChannel:
		return p.ChannelID
	}
	return 0
}

func (c *GotdClient) cachePeer(p store.Peer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.peers[p.ID] = p
}

func convertUser(u *tg.User) (store.Chat, bool) {
	if u == nil {
		return store.Chat{}, false
	}
	var name string
	if u.Self {
		name = "Saved Messages"
	} else {
		name = strings.TrimSpace(u.FirstName + " " + u.LastName)
		if name == "" {
			name = fmt.Sprintf("User %d", u.ID)
		}
	}
	_, isOnline := u.Status.(*tg.UserStatusOnline)
	return store.Chat{
		ID:        u.ID,
		Title:     name,
		Peer:      store.Peer{ID: u.ID, Type: store.PeerUser, AccessHash: u.AccessHash},
		IsContact: u.Contact,
		IsBot:     u.Bot,
		Online:    isOnline,
	}, true
}

func convertGroupChat(ch *tg.Chat) (store.Chat, bool) {
	if ch == nil {
		return store.Chat{}, false
	}
	return store.Chat{
		ID:    ch.ID,
		Title: ch.Title,
		Peer:  store.Peer{ID: ch.ID, Type: store.PeerGroup},
	}, true
}

func convertChannel(ch *tg.Channel) (store.Chat, bool) {
	if ch == nil {
		return store.Chat{}, false
	}
	peerType := store.PeerChannel
	if ch.Megagroup {
		peerType = store.PeerSuperGroup
	}
	return store.Chat{
		ID:    ch.ID,
		Title: ch.Title,
		Peer:  store.Peer{ID: ch.ID, Type: peerType, AccessHash: ch.AccessHash},
	}, true
}

func isMuted(d *tg.Dialog) bool {
	return mutedFromSettings(d.NotifySettings)
}

// mutedFromSettings reports whether the given notify settings mean the peer is
// currently muted. Shared by initial dialog parsing and live UpdateNotifySettings.
func mutedFromSettings(s tg.PeerNotifySettings) bool {
	muteUntil, hasMute := s.GetMuteUntil()
	if !hasMute {
		return false
	}
	const mutedIndefinitely = math.MaxInt32
	if muteUntil == mutedIndefinitely {
		return true
	}
	return muteUntil > int(time.Now().Unix())
}
