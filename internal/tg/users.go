package tg

import (
	"context"
	"strings"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// UserAddress is how a user is named to Telegram. A person the account has a
// dialog with is addressed directly by their access hash; a person met only in
// a group is addressed through a message they wrote there, which is the only
// address available when no dialog exists and no hash was ever stored (#222).
//
// The caller fills in whichever it has: a non-zero AccessHash wins, and
// FromChat/FromMsgID are the fallback.
type UserAddress struct {
	UserID     int64
	AccessHash int64
	FromChat   domain.Peer
	FromMsgID  int
}

// inputUser builds the Telegram constructor for the address, or reports that
// the address names nobody reachable.
func (a UserAddress) inputUser() (tg.InputUserClass, error) {
	if a.UserID == 0 {
		return nil, &telerr.Error{Kind: telerr.PeerNotFound, Op: "get user"}
	}
	if a.AccessHash != 0 {
		return &tg.InputUser{UserID: a.UserID, AccessHash: a.AccessHash}, nil
	}
	if a.FromMsgID != 0 && a.FromChat.ID != 0 {
		return &tg.InputUserFromMessage{
			Peer:   peerToInput(a.FromChat),
			MsgID:  a.FromMsgID,
			UserID: a.UserID,
		}, nil
	}
	return nil, &telerr.Error{Kind: telerr.PeerNotFound, Op: "get user"}
}

// inputPeer builds the peer constructor for the address. It exists alongside
// inputUser because a file location names a peer rather than a user: an avatar
// is downloaded through inputPeerPhotoFileLocation, which takes an InputPeer
// (#223). The two forms mirror inputUser's exactly, so a person met only in a
// group is reachable here as well.
func (a UserAddress) inputPeer() (tg.InputPeerClass, error) {
	if a.UserID == 0 {
		return nil, &telerr.Error{Kind: telerr.PeerNotFound, Op: "download avatar"}
	}
	if a.AccessHash != 0 {
		return &tg.InputPeerUser{UserID: a.UserID, AccessHash: a.AccessHash}, nil
	}
	if a.FromMsgID != 0 && a.FromChat.ID != 0 {
		return &tg.InputPeerUserFromMessage{
			Peer:   peerToInput(a.FromChat),
			MsgID:  a.FromMsgID,
			UserID: a.UserID,
		}, nil
	}
	return nil, &telerr.Error{Kind: telerr.PeerNotFound, Op: "download avatar"}
}

// GetUser fetches a user's full profile via users.getFullUser.
//
// The response carries the person twice: the short tg.User in Users holds the
// name, the username and the presence, while UserFull holds the about text and
// the phone. Both are folded into one domain.User, because the split is
// Telegram's rather than the domain's.
func (c *GotdClient) GetUser(ctx context.Context, addr UserAddress) (domain.User, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return domain.User{}, err
	}
	input, err := addr.inputUser()
	if err != nil {
		return domain.User{}, err
	}
	var out domain.User
	err = WithRetry(ctx, func() error {
		full, err := api.UsersGetFullUser(ctx, input)
		if err != nil {
			c.log.Error("UsersGetFullUser failed", zap.Error(err), zap.Int64("user_id", addr.UserID))
			return err
		}
		out = buildUser(addr.UserID, full)
		return nil
	})
	if err != nil {
		return domain.User{}, err
	}
	return out, nil
}

// buildUser folds a users.getFullUser response into a domain.User. A response
// whose Users slice does not carry the person asked about still yields what
// UserFull knows: a partial answer beats none, and the caller already holds a
// name.
func buildUser(userID int64, full *tg.UsersUserFull) domain.User {
	out := domain.User{ID: userID}
	if full == nil {
		return out
	}
	for _, uc := range full.Users {
		u, ok := uc.(*tg.User)
		if !ok || u.ID != userID {
			continue
		}
		_, isOnline := u.Status.(*tg.UserStatusOnline)
		out.Username = u.Username
		out.FirstName = u.FirstName
		out.LastName = u.LastName
		out.Online = isOnline
		out.IsBot = u.Bot
		out.IsContact = u.Contact
		out.IsMutualContact = u.MutualContact
		out.IsDeleted = u.Deleted
		// Telegram returns the phone on the short user, and only when privacy
		// permits it. An empty string is the ordinary case, not a failure.
		out.Phone = strings.TrimSpace(u.Phone)
		// A person with no avatar, and one whose privacy settings withhold it,
		// both arrive as userProfilePhotoEmpty (or no photo at all), which is
		// why neither is distinguished here: there is nothing to download in
		// either case, and the client draws the same monogram for both (#223).
		if photo, ok := u.Photo.(*tg.UserProfilePhoto); ok {
			out.AvatarID = photo.PhotoID
		}
		break
	}
	if about, ok := full.FullUser.GetAbout(); ok {
		out.Bio = strings.TrimSpace(about)
	}
	return out
}
