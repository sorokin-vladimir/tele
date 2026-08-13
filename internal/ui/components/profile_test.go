package components_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func newProfile(u domain.User, hasDialog, muted bool) *components.Profile {
	return components.NewProfile(u, hasDialog, muted, defaultKM(), 100, 40)
}

func alice() domain.User {
	return domain.User{ID: 7, FirstName: "Alice", LastName: "Ng", Username: "alice"}
}

// --- identity block ---

func TestProfile_ShowsNameAndHandle(t *testing.T) {
	p := newProfile(alice(), true, false)
	view := strip(p.View())
	assert.Contains(t, view, "Alice Ng")
	assert.Contains(t, view, "@alice")
}

func TestProfile_ShowsOnlineOnlyWhenOnline(t *testing.T) {
	u := alice()
	u.Online = true
	assert.Contains(t, strip(newProfile(u, true, false).View()), "online")

	// Presence beyond "online" is #127. Absent presence means unknown, so the
	// overlay says nothing rather than claiming the person is away.
	u.Online = false
	assert.NotContains(t, strip(newProfile(u, true, false).View()), "online")
	assert.NotContains(t, strip(newProfile(u, true, false).View()), "offline")
}

func TestProfile_NameFallsBackToID(t *testing.T) {
	p := newProfile(domain.User{ID: 42}, false, false)
	assert.Contains(t, strip(p.View()), "User 42")
}

func TestProfile_ShowsPhoneWithPlus(t *testing.T) {
	u := alice()
	u.Phone = "79991234567"
	p := newProfile(u, true, false)
	p.SetUser(u)
	assert.Contains(t, strip(p.View()), "+79991234567")
}

// --- partial profile ---

func TestProfile_PartialSaysSoAndCompletes(t *testing.T) {
	p := newProfile(domain.User{ID: 7, FirstName: "Alice"}, true, false)
	// Before the full answer, an empty bio means "not known yet"; the overlay
	// must not read as a person with nothing to say.
	assert.Contains(t, strip(p.View()), "…")

	full := alice()
	full.Bio = "builds terminals"
	p.SetUser(full)
	view := strip(p.View())
	assert.Contains(t, view, "builds terminals")
	assert.NotContains(t, view, "…")
}

func TestProfile_CompletedWithNothingToShowDropsThePlaceholder(t *testing.T) {
	p := newProfile(domain.User{ID: 7, FirstName: "Alice"}, true, false)
	p.SetUser(domain.User{ID: 7, FirstName: "Alice"})
	assert.NotContains(t, strip(p.View()), "…")
}

func TestProfile_CompletionKeepsTheCursor(t *testing.T) {
	p := newProfile(alice(), true, false)
	p, _ = p.Update(pressJ())
	require.NotNil(t, p)
	before := p.Cursor()
	p.SetUser(alice())
	assert.Equal(t, before, p.Cursor(), "an answer arriving must not move the selection")
}

// --- which actions exist ---

func TestProfile_NoDialog_HasNoMuteItem(t *testing.T) {
	view := strip(newProfile(alice(), false, false).View())
	assert.NotContains(t, view, "Mute")
	assert.NotContains(t, view, "Unmute")
	assert.Contains(t, view, "Open chat", "open chat works with no dialog too")
}

func TestProfile_WithDialog_ShowsMuteOrUnmute(t *testing.T) {
	assert.Contains(t, strip(newProfile(alice(), true, false).View()), "Mute")
	assert.Contains(t, strip(newProfile(alice(), true, true).View()), "Unmute")
}

func TestProfile_NoUsername_HasNoCopyItem(t *testing.T) {
	u := alice()
	u.Username = ""
	assert.NotContains(t, strip(newProfile(u, true, false).View()), "Copy username")
}

// --- actions ---

func TestProfile_OpenChat_EmitsRequestAndCloses(t *testing.T) {
	p := newProfile(alice(), true, false)
	next, cmd := p.Update(pressO())
	assert.Nil(t, next, "opening a chat takes you elsewhere, so the overlay goes")
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ProfileOpenChatRequest)
	require.True(t, ok)
	assert.Equal(t, int64(7), req.UserID)
}

func TestProfile_Mute_FlipsItsOwnRowAndStaysOpen(t *testing.T) {
	p := newProfile(alice(), true, false)
	next, cmd := p.Update(pressM())
	require.NotNil(t, next, "muting is a toggle you watch land; the overlay stays")
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ProfileMuteRequest)
	require.True(t, ok)
	assert.Equal(t, int64(7), req.UserID)
	assert.True(t, req.Muted)
	// The overlay is a snapshot: it flips its own row rather than waiting for a
	// delta it does not listen to.
	assert.Contains(t, strip(next.View()), "Unmute")
}

func TestProfile_Unmute_SendsMutedFalse(t *testing.T) {
	p := newProfile(alice(), true, true)
	next, cmd := p.Update(pressM())
	require.NotNil(t, next)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ProfileMuteRequest)
	require.True(t, ok)
	assert.False(t, req.Muted)
	assert.Contains(t, strip(next.View()), "Mute")
}

func TestProfile_CopyUsername_CarriesTheAt(t *testing.T) {
	p := newProfile(alice(), true, false)
	next, cmd := p.Update(keyMsg('y'))
	assert.Nil(t, next)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ProfileCopyUsernameRequest)
	require.True(t, ok)
	assert.Equal(t, "@alice", req.Username)
}

func TestProfile_Esc_Closes(t *testing.T) {
	p := newProfile(alice(), true, false)
	next, cmd := p.Update(pressEsc())
	assert.Nil(t, next)
	require.NotNil(t, cmd)
	assert.IsType(t, components.CloseProfileMsg{}, cmd())
}

func TestProfile_MuteKeyOnAProfileWithoutADialog_DoesNothing(t *testing.T) {
	p := newProfile(alice(), false, false)
	next, cmd := p.Update(pressM())
	require.NotNil(t, next)
	assert.Nil(t, cmd, "an absent item cannot be fired by its key either")
}

// --- geometry ---

func TestProfile_TooNarrowBelowTheMinimum(t *testing.T) {
	narrow := components.NewProfile(alice(), true, false, defaultKM(), components.ProfileMinWidth-1, 40)
	assert.True(t, narrow.TooNarrow())
	assert.Empty(t, narrow.View())

	wide := components.NewProfile(alice(), true, false, defaultKM(), components.ProfileMinWidth, 40)
	assert.False(t, wide.TooNarrow())
}

func TestProfile_NeverWiderThanTheTerminal(t *testing.T) {
	u := alice()
	u.FirstName = "Alice with a preposterously long name that will not fit"
	u.Bio = "and a bio to match, going on at some length about nothing at all"
	p := components.NewProfile(u, true, false, defaultKM(), 40, 20)
	for _, line := range strings.Split(strip(p.View()), "\n") {
		assert.LessOrEqual(t, len([]rune(line)), 40, "the overlay must fit the terminal")
	}
}

// --- entry points in the menus ---

func TestMessageMenu_ProfileItem_OnlyWithAnAuthor(t *testing.T) {
	withSender := components.NewContextMenu(1, false, 9, 0, 0, false, false, nil, defaultKM())
	assert.Contains(t, strip(withSender.View()), "Profile")

	// An outgoing message names no other person, so there is nobody to look at.
	own := components.NewContextMenu(1, true, 0, 0, 0, false, false, nil, defaultKM())
	assert.NotContains(t, strip(own.View()), "Profile")
}

func TestMessageMenu_Profile_EmitsOpenProfileRequest(t *testing.T) {
	cm := components.NewContextMenu(1, false, 9, 0, 0, false, false, nil, defaultKM())
	next, cmd := cm.Update(keyMsg('P'))
	assert.Nil(t, next)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.OpenProfileRequest)
	require.True(t, ok)
	assert.Equal(t, int64(9), req.UserID)
}

func TestChatMenu_ProfileItem_OnlyForAPrivateChat(t *testing.T) {
	user := domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}}
	assert.Contains(t, strip(components.NewChatContextMenu(user, nil, defaultKM()).View()), "Profile")

	group := domain.Chat{ID: 5, Title: "Group", Peer: domain.Peer{ID: 5, Type: domain.PeerSuperGroup}}
	assert.NotContains(t, strip(components.NewChatContextMenu(group, nil, defaultKM()).View()), "Profile")
}

func TestChatMenu_Profile_EmitsOpenProfileRequest(t *testing.T) {
	user := domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}}
	cm := components.NewChatContextMenu(user, nil, defaultKM())
	next, cmd := cm.Update(keyMsg('P'))
	assert.Nil(t, next)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.OpenProfileRequest)
	require.True(t, ok)
	assert.Equal(t, int64(1), req.UserID)
}
