package ui_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui"
)

// avatarPNG writes a decodable square image and returns its path, standing in
// for what the owner's cache hands back.
func avatarPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 160, 160))
	for y := 0; y < 160; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	path := filepath.Join(t.TempDir(), "avatar.png")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
	return path
}

// kittyRoot is a model on a chat-list row for Ada, in a terminal that can draw
// images. Avatars are only ever fetched there.
func kittyRoot(t *testing.T) ui.RootModel {
	t.Helper()
	cfg := &config.Config{}
	cfg.Photos.Mode = "kitty"
	m := rootOnChatList(t, domain.Chat{
		ID: 7, Title: "Ada", Peer: domain.Peer{ID: 7, Type: domain.PeerUser},
	})
	return m.WithConfig(cfg)
}

// ada is the person the profile is opened on, with a picture.
func ada() domain.User {
	return domain.User{ID: 7, FirstName: "Ada", LastName: "Lovelace", AvatarID: 4242}
}

// openAdaProfile opens the overlay and delivers the full profile, which is the
// only thing that ever carries an avatar id.
func openAdaProfile(t *testing.T, m ui.RootModel, full domain.User) (ui.RootModel, tea.Cmd) {
	t.Helper()
	o := ownerOf(t, m)
	o.knownUsers = map[int64]domain.User{7: {ID: 7, FirstName: "Ada"}}
	nm, _ := m.Update(pressProfileKey())
	m = nm.(ui.RootModel)
	require.True(t, m.ProfileOpen())
	nm, cmd := m.Update(ui.ProfileLoadedMsgForTest(7, full))
	return nm.(ui.RootModel), cmd
}

func TestAvatar_FullProfileTriggersTheFetch(t *testing.T) {
	m := kittyRoot(t)
	o := ownerOf(t, m)
	o.avatarPaths[avatarPathKey{userID: 7, avatarID: 4242}] = avatarPNG(t)

	_, cmd := openAdaProfile(t, m, ada())
	runCmds(cmd)

	assert.Equal(t, []avatarPathKey{{userID: 7, avatarID: 4242}}, o.avatarsFetched)
}

// A person with no picture, and one whose privacy withholds it, both arrive as
// AvatarID 0. Neither is worth a round trip: the monogram is the answer.
func TestAvatar_NoAvatarIDIsNotFetched(t *testing.T) {
	m := kittyRoot(t)
	o := ownerOf(t, m)
	u := ada()
	u.AvatarID = 0

	_, cmd := openAdaProfile(t, m, u)
	runCmds(cmd)

	assert.Empty(t, o.avatarsFetched)
}

// A terminal that cannot draw an image is never sent one: the monogram is what
// it shows either way, so the download would buy nothing.
func TestAvatar_BlocksModeFetchesNothing(t *testing.T) {
	cfg := &config.Config{}
	cfg.Photos.Mode = "blocks"
	m := rootOnChatList(t, domain.Chat{
		ID: 7, Title: "Ada", Peer: domain.Peer{ID: 7, Type: domain.PeerUser},
	}).WithConfig(cfg)
	o := ownerOf(t, m)
	o.avatarPaths[avatarPathKey{userID: 7, avatarID: 4242}] = avatarPNG(t)

	_, cmd := openAdaProfile(t, m, ada())
	runCmds(cmd)

	assert.Empty(t, o.avatarsFetched)
}

// The second opening draws the remembered picture at once, without waiting for
// the profile and without downloading anything again.
func TestAvatar_SecondOpeningReusesTheRememberedPicture(t *testing.T) {
	m := kittyRoot(t)
	o := ownerOf(t, m)
	o.avatarPaths[avatarPathKey{userID: 7, avatarID: 4242}] = avatarPNG(t)

	m, cmd := openAdaProfile(t, m, ada())
	for _, msg := range drainMsgs(cmdMsg(cmd)) {
		nm, _ := m.Update(msg)
		m = nm.(ui.RootModel)
	}
	require.Len(t, o.avatarsFetched, 1)

	// Close, then open again. The overlay has the picture before anything is
	// known about whether it is still current.
	nm, _ := m.Update(pressEsc())
	m = nm.(ui.RootModel)
	require.False(t, m.ProfileOpen())
	nm, _ = m.Update(pressProfileKey())
	m = nm.(ui.RootModel)
	require.True(t, m.ProfileOpen())
	assert.True(t, m.Profile().HasAvatar(), "the remembered face is drawn immediately")

	// And the confirming profile does not start a second download.
	_, cmd2 := m.Update(ui.ProfileLoadedMsgForTest(7, ada()))
	runCmds(cmd2)
	assert.Len(t, o.avatarsFetched, 1, "the same id means the remembered picture is current")
}

// A person who changed their picture has a different id, and that alone is what
// makes the client fetch again — nothing subscribes to anything.
func TestAvatar_AChangedIDIsFetchedAgain(t *testing.T) {
	m := kittyRoot(t)
	o := ownerOf(t, m)
	o.avatarPaths[avatarPathKey{userID: 7, avatarID: 4242}] = avatarPNG(t)
	o.avatarPaths[avatarPathKey{userID: 7, avatarID: 5555}] = avatarPNG(t)

	m, cmd := openAdaProfile(t, m, ada())
	for _, msg := range drainMsgs(cmdMsg(cmd)) {
		nm, _ := m.Update(msg)
		m = nm.(ui.RootModel)
	}

	changed := ada()
	changed.AvatarID = 5555
	_, cmd2 := m.Update(ui.ProfileLoadedMsgForTest(7, changed))
	runCmds(cmd2)

	assert.Equal(t, []avatarPathKey{
		{userID: 7, avatarID: 4242},
		{userID: 7, avatarID: 5555},
	}, o.avatarsFetched)
}

// An answer about someone whose profile has been closed is not drawn, but it is
// still remembered: the download already happened.
func TestAvatar_AnswerForAClosedProfileIsKeptNotDrawn(t *testing.T) {
	m := kittyRoot(t)
	o := ownerOf(t, m)
	o.avatarPaths[avatarPathKey{userID: 7, avatarID: 4242}] = avatarPNG(t)

	m, _ = openAdaProfile(t, m, ada())
	nm, _ := m.Update(pressEsc())
	m = nm.(ui.RootModel)
	require.False(t, m.ProfileOpen())

	img := image.NewRGBA(image.Rect(0, 0, 160, 160))
	nm, _ = m.Update(ui.AvatarReadyMsgForTest(7, 4242, img))
	m = nm.(ui.RootModel)

	nm, _ = m.Update(pressProfileKey())
	m = nm.(ui.RootModel)
	assert.True(t, m.Profile().HasAvatar(), "the picture that arrived late is not thrown away")
}

// An undecodable file is dropped from the owner's cache, so the next fetch
// downloads it again instead of getting the same bytes back.
func TestAvatar_UndecodableBytesAreInvalidated(t *testing.T) {
	m := kittyRoot(t)
	o := ownerOf(t, m)
	broken := filepath.Join(t.TempDir(), "broken.png")
	require.NoError(t, os.WriteFile(broken, []byte("not an image"), 0o600))
	o.avatarPaths[avatarPathKey{userID: 7, avatarID: 4242}] = broken

	_, cmd := openAdaProfile(t, m, ada())
	runCmds(cmd)

	assert.Equal(t, []avatarPathKey{{userID: 7, avatarID: 4242}}, o.avatarsInvalidated)
}

func pressEsc() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEscape} }

// runCmds runs a command and everything it batches, so a test can assert on
// what the commands did rather than on their shape.
func runCmds(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			runCmds(c)
		}
	}
}

// cmdMsg runs one command, returning nil for none.
func cmdMsg(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}
