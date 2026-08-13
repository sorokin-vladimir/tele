package tg

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// fullUserResponse wraps a short user in the shape users.getFullUser answers
// with, so the tests read as "Telegram said this".
func fullUserResponse(u *tg.User) *tg.UsersUserFull {
	out := &tg.UsersUserFull{}
	if u != nil {
		out.Users = []tg.UserClass{u}
	}
	return out
}

func TestBuildUser_TakesTheAvatarIDFromTheProfilePhoto(t *testing.T) {
	got := buildUser(7, fullUserResponse(&tg.User{
		ID:        7,
		FirstName: "Ada",
		Photo:     &tg.UserProfilePhoto{PhotoID: 4242},
	}))

	assert.Equal(t, int64(4242), got.AvatarID)
}

// A person who set no picture, and one whose privacy withholds it, arrive the
// same way and mean the same thing here: nothing to fetch.
func TestBuildUser_NoAvatarIsZero(t *testing.T) {
	empty := buildUser(7, fullUserResponse(&tg.User{
		ID:    7,
		Photo: &tg.UserProfilePhotoEmpty{},
	}))
	absent := buildUser(7, fullUserResponse(&tg.User{ID: 7}))

	assert.Zero(t, empty.AvatarID)
	assert.Zero(t, absent.AvatarID)
}

// A response that carries no short user still yields what UserFull knows; there
// is simply no picture in it.
func TestBuildUser_AResponseWithoutTheShortUserHasNoAvatar(t *testing.T) {
	got := buildUser(7, fullUserResponse(nil))

	assert.Equal(t, int64(7), got.ID)
	assert.Zero(t, got.AvatarID)
}

func TestUserAddress_InputPeerPrefersTheAccessHash(t *testing.T) {
	addr := UserAddress{
		UserID:     7,
		AccessHash: 99,
		FromChat:   domain.Peer{ID: 50, Type: domain.PeerGroup},
		FromMsgID:  3,
	}

	peer, err := addr.inputPeer()

	require.NoError(t, err)
	require.IsType(t, &tg.InputPeerUser{}, peer)
	assert.Equal(t, int64(99), peer.(*tg.InputPeerUser).AccessHash)
}

// Someone met only in a group has no hash anywhere on the account, and is
// reachable only through a message they wrote (#222, ADR 0006).
func TestUserAddress_InputPeerFallsBackToAMessage(t *testing.T) {
	addr := UserAddress{
		UserID:    7,
		FromChat:  domain.Peer{ID: 50, Type: domain.PeerGroup},
		FromMsgID: 3,
	}

	peer, err := addr.inputPeer()

	require.NoError(t, err)
	require.IsType(t, &tg.InputPeerUserFromMessage{}, peer)
	from := peer.(*tg.InputPeerUserFromMessage)
	assert.Equal(t, int64(7), from.UserID)
	assert.Equal(t, 3, from.MsgID)
}

func TestUserAddress_InputPeerRefusesAnAddressThatNamesNobody(t *testing.T) {
	_, noHash := UserAddress{UserID: 7}.inputPeer()
	_, noUser := UserAddress{AccessHash: 99}.inputPeer()

	assert.Error(t, noHash)
	assert.Error(t, noUser)
}
