package core

import (
	"context"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// SearchContacts asks Telegram for users matching q. It is a query, not a
// projection: the result is a one-off answer nobody subscribes to. No chat ID
// is involved because the point is finding chats the owner may not hold.
//
// Every hit's address is kept, since a hit the user opens and writes to is
// named by id from then on and search is the only place its address came from.
// The client gets rows, which carry no address (#278).
func (o *Owner) SearchContacts(ctx context.Context, q string, limit int) ([]project.ChatRow, error) {
	chats, err := o.client.SearchContacts(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	rows := make([]project.ChatRow, 0, len(chats))
	for _, c := range chats {
		o.state.RememberAddress(c.Peer)
		rows = append(rows, project.Row(c))
	}
	return rows, nil
}

// Chats answers with every chat the owner holds, archived ones included, in
// the order the chat list shows them. It is a query for a modal - search, the
// forward picker - that looks at the whole list for a few seconds; a pane that
// stays on screen subscribes to a window instead.
func (o *Owner) Chats(_ context.Context) ([]project.ChatRow, error) {
	chats := o.state.Store().Chats()
	rows := make([]project.ChatRow, 0, len(chats))
	for _, c := range chats {
		rows = append(rows, project.Row(c))
	}
	return rows, nil
}

// Chat answers with one chat the owner holds, and whether it holds it. A person
// the owner can only address - a search hit, a profile opened from a group - is
// not a chat until a dialog exists.
func (o *Owner) Chat(_ context.Context, chatID int64) (project.ChatRow, bool, error) {
	c, ok := o.state.Store().GetChat(chatID)
	if !ok {
		return project.ChatRow{}, false, nil
	}
	return project.Row(c), true, nil
}

// FolderFilters answers with the folders the owner already holds. It does not
// go to Telegram: LoadFolderFilters is the refresh, and it runs once at start.
func (o *Owner) FolderFilters(_ context.Context) ([]domain.FolderFilter, error) {
	return o.state.Store().FolderFilters(), nil
}

// GetParticipants returns mention candidates for a group or channel.
func (o *Owner) GetParticipants(ctx context.Context, chatID int64) ([]domain.ChatMember, error) {
	peer, err := o.peer(chatID)
	if err != nil {
		return nil, err
	}
	return o.client.GetParticipants(ctx, peer)
}

// KnownUser answers who a user is from what the owner already holds, with no
// round trip. It is deliberately synchronous and lossy: a client opening a
// profile draws this immediately and lets GetUser complete it, so the name that
// was already on screen does not blink out (#222).
func (o *Owner) KnownUser(userID int64) (domain.User, bool) {
	st := o.state.Store()
	if chat, ok := st.GetChat(userID); ok && chat.Peer.IsUser() {
		return domain.User{
			ID: userID,
			// The dialog knows one combined name, not two fields. It goes in
			// FirstName whole rather than being split on a space, which would
			// invent a surname; getFullUser replaces both.
			FirstName: chat.Title,
			Online:    chat.Online,
			IsBot:     chat.IsBot,
			IsContact: chat.IsContact,
		}, true
	}
	if _, msg, ok := o.findMessageFrom(userID); ok {
		return domain.User{ID: userID, FirstName: msg.SenderName}, true
	}
	return domain.User{}, false
}

// GetUser fetches a user's full profile. Addressing is the owner's business:
// the client passes an id and never a hash, because for the entry point that
// matters most — the author of a message in a group — it holds no hash to pass
// (#222, ADR 0006).
func (o *Owner) GetUser(ctx context.Context, userID int64) (domain.User, error) {
	addr, err := o.userAddress(userID)
	if err != nil {
		return domain.User{}, err
	}
	full, err := o.client.GetUser(ctx, addr)
	if err != nil {
		return domain.User{}, err
	}
	// A profile opened from a group is the other way to find someone the
	// account has no dialog with, and a first message to them needs this.
	o.state.RememberAddress(full.Peer)
	user := full.User
	// A response that carried no short user has no name in it. Rather than
	// hand back a nameless profile, keep the one already on screen.
	if user.FirstName == "" && user.LastName == "" {
		if known, ok := o.KnownUser(userID); ok {
			user.FirstName, user.LastName = known.FirstName, known.LastName
		}
	}
	return user, nil
}

// FetchAvatar downloads the person's avatar into the owner's cache if it is not
// there already, and returns the path. Like FetchMedia, the client decodes the
// file and the bytes never cross the owner boundary (#196).
//
// avatarID is the id the client was handed in domain.User. Passing it back is
// what keeps a changed picture from being served forever: a new avatar is a new
// id and therefore a different cache entry. Addressing stays here — the client
// holds no access hash for someone met in a group (ADR 0006).
func (o *Owner) FetchAvatar(ctx context.Context, userID, avatarID int64) (string, error) {
	addr, err := o.userAddress(userID)
	if err != nil {
		return "", err
	}
	return o.avatars.Fetch(ctx, addr, avatarID)
}

// InvalidateAvatar drops a cached avatar a client could not decode, so the next
// fetch downloads it again rather than handing back the same broken entry.
func (o *Owner) InvalidateAvatar(userID, avatarID int64) {
	o.avatars.Invalidate(userID, avatarID)
}

// userAddress works out how a user can be named to Telegram: by the access hash
// of their dialog when one exists, then by one Telegram handed out earlier, and
// otherwise through a message they wrote, which is the only address left for
// someone met in a group.
func (o *Owner) userAddress(userID int64) (internaltg.UserAddress, error) {
	if userID == 0 {
		return internaltg.UserAddress{}, &telerr.Error{Kind: telerr.PeerNotFound, Op: "get user"}
	}
	st := o.state.Store()
	if chat, ok := st.GetChat(userID); ok && chat.Peer.IsUser() && chat.Peer.AccessHash != 0 {
		return internaltg.UserAddress{UserID: userID, AccessHash: chat.Peer.AccessHash}, nil
	}
	if p, ok := st.Address(userID); ok && p.IsUser() {
		return internaltg.UserAddress{UserID: userID, AccessHash: p.AccessHash}, nil
	}
	if peer, msg, ok := o.findMessageFrom(userID); ok {
		return internaltg.UserAddress{UserID: userID, FromChat: peer, FromMsgID: msg.ID}, nil
	}
	return internaltg.UserAddress{}, &telerr.Error{Kind: telerr.PeerNotFound, Op: "get user"}
}

// findMessageFrom looks for any message the user wrote in a chat the owner
// holds. It scans the recent tail the store keeps in memory, newest chats
// first, and stops at the first hit — a profile is opened rarely, and the chat
// the person was seen in is usually the open one.
func (o *Owner) findMessageFrom(userID int64) (domain.Peer, domain.Message, bool) {
	st := o.state.Store()
	for _, chat := range st.Chats() {
		msgs := st.Messages(chat.ID)
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].SenderID == userID && msgs[i].ID != 0 {
				return chat.Peer, msgs[i], true
			}
		}
	}
	return domain.Peer{}, domain.Message{}, false
}
