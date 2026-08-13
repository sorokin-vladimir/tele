package ui

import (
	"image"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// profileLoadedMsg carries the completed answer back to an open overlay. It
// names the user it is about because the overlay may have been closed and
// another opened while the query was in flight.
type profileLoadedMsg struct {
	userID int64
	user   domain.User
}

// ProfileLoadedMsgForTest builds the completion message, so a test can deliver
// an answer without reaching into the unexported type.
func ProfileLoadedMsgForTest(userID int64, user domain.User) tea.Msg {
	return profileLoadedMsg{userID: userID, user: user}
}

// avatarReadyMsg carries a decoded avatar back. It names both the person and
// the picture: the person because the overlay may have been closed and another
// opened, and the picture because the person may have changed it while this one
// was in flight.
type avatarReadyMsg struct {
	userID   int64
	avatarID int64
	img      image.Image
}

// AvatarReadyMsgForTest builds the completion message for tests in other
// packages.
func AvatarReadyMsgForTest(userID, avatarID int64, img image.Image) tea.Msg {
	return avatarReadyMsg{userID: userID, avatarID: avatarID, img: img}
}

// profileTargetUserID picks the person the focused pane is currently about: the
// author of the selected message when one is selected, and otherwise the person
// on the other side of an open private chat. 0 when neither applies.
//
// This is the key choosing which id to hand over, not the overlay learning
// where it was opened from — the overlay is handed an id and nothing else.
func (m RootModel) profileTargetUserID() int64 {
	switch m.focus {
	case FocusChat:
		if m.chat == nil {
			return 0
		}
		if id := m.chat.SelectedMessageSenderID(); id != 0 {
			return id
		}
		return m.chat.PeerUserID()
	case FocusChatList:
		if row, ok := m.chatList.CursorChat(); ok && row.IsUser {
			return row.ID
		}
	}
	return 0
}

// openProfile builds the overlay from what the owner already knows and asks for
// the rest. A person the owner knows nothing about opens nothing: there is no
// profile to draw and no id to complete.
func (m RootModel) openProfile(userID int64) (RootModel, tea.Cmd) {
	if userID == 0 || m.owner == nil {
		return m, nil
	}
	user, ok := m.owner.KnownUser(userID)
	if !ok {
		m.statusBar.SetStatus("No profile for this user")
		return m, nil
	}
	// A dialog is what mute is about. The client's own store answers it, which
	// is also what the user sees: a chat in the list has a mute item, and a
	// person never messaged has none rather than a guessed one.
	hasDialog, muted := false, false
	if m.st != nil {
		if chat, found := m.st.GetChat(userID); found && chat.Peer.IsUser() {
			hasDialog, muted = true, chat.IsMuted
		}
	}
	// Opening the profile closes the menu it was opened from: two overlays
	// competing for the same keys is one too many.
	m.contextMenu = nil
	m.chatMenu = nil
	p := components.NewProfile(user, hasDialog, muted, m.keyMap, m.width, m.height)
	if p.TooNarrow() {
		m.statusBar.SetStatus("Terminal too narrow for the profile")
		return m, nil
	}
	m.profile = p

	// What this person looked like last time is drawn at once, before anything
	// is known about whether it is still current: the answer arriving a moment
	// later either confirms it or replaces it, and a face that was right a
	// minute ago beats letters while we wait (#223).
	var cmds []tea.Cmd
	if m.imageMode == media.ModeKitty {
		m.profile.SetRenderer(media.NewKittyRenderer(m.kittyStore))
		if entry, ok := m.avatars.Get(userID); ok {
			m.profile.SetAvatar(entry.img)
			cmds = append(cmds, m.transmitAvatarCmd(entry.img))
		}
	}

	ctx, owner := m.ctx, m.owner
	cmds = append(cmds, func() tea.Msg {
		full, err := owner.GetUser(ctx, userID)
		if err != nil {
			// A profile that could not be completed stays as it opened. The
			// name was already on screen; saying so again would be noise.
			return nil
		}
		return profileLoadedMsg{userID: userID, user: full}
	})
	return m, tea.Batch(cmds...)
}

// transmitAvatarCmd encodes the picture for the terminal under the overlay's
// stable image id. Only ever called in Kitty mode.
//
// It goes through kittyEncodedMsg like every other image rather than writing
// the sequence itself: that path writes the placement first and only then marks
// it ready, so the overlay keeps drawing the monogram until the picture really
// is on the terminal, and a failed encode never marks anything ready (#95).
func (m RootModel) transmitAvatarCmd(img image.Image) tea.Cmd {
	cols, rows := components.AvatarBox()
	id := m.kittyStore.IDFor(components.ProfileAvatarImageKey)
	return func() tea.Msg {
		seq, err := media.TransmitSeq(id, img, cols, rows)
		if err != nil {
			return nil
		}
		return kittyEncodedMsg{photoID: components.ProfileAvatarImageKey, cols: cols, seq: seq}
	}
}

// handleProfileLoaded completes an open overlay, ignoring an answer for a
// profile that has since been closed or replaced.
func (m RootModel) handleProfileLoaded(msg profileLoadedMsg) (RootModel, tea.Cmd) {
	if m.profile == nil || m.profile.UserID() != msg.userID {
		return m, nil
	}
	m.profile.SetUser(msg.user)
	return m, m.avatarCmdFor(msg.userID, msg.user.AvatarID)
}

// avatarCmdFor asks for the person's picture, unless there is nothing to ask
// for or nothing would come of it:
//
//   - a terminal that cannot draw images is never sent one, because the
//     monogram is what it will show either way;
//   - AvatarID 0 means the person set no avatar or their privacy withholds it,
//     which is a whole answer rather than a failed fetch;
//   - a picture already on screen under the same id is the current one, and the
//     id changing is precisely how a changed avatar announces itself (#223).
func (m RootModel) avatarCmdFor(userID, avatarID int64) tea.Cmd {
	if m.imageMode != media.ModeKitty || m.owner == nil || avatarID == 0 {
		return nil
	}
	if entry, ok := m.avatars.Get(userID); ok && entry.avatarID == avatarID {
		return nil
	}
	return fetchAvatarCmd(m.ctx, m.owner, userID, avatarID)
}

// handleAvatarReady records the picture and shows it, if the overlay is still
// about this person. The store is written either way: the download already
// happened, and the next opening should not repeat it.
func (m RootModel) handleAvatarReady(msg avatarReadyMsg) (RootModel, tea.Cmd) {
	m.avatars.Add(msg.userID, msg.avatarID, msg.img)
	if m.profile == nil || m.profile.UserID() != msg.userID {
		return m, nil
	}
	m.profile.SetAvatar(msg.img)
	return m, m.transmitAvatarCmd(msg.img)
}

// closeProfile tears down the overlay and, in Kitty mode, deletes the avatar's
// placement from the terminal. The id is reused by every profile, so leaving
// the placement alive lets the next person's placeholder cells resolve against
// this one's geometry and draw the wrong face at the wrong size (#175).
// It deliberately does not require the overlay to still be there: a key press
// drops it the moment it is handled, and the close message arrives after, so
// checking would skip the delete in the common case.
func (m RootModel) closeProfile() (RootModel, tea.Cmd) {
	m.profile = nil
	cols, _ := components.AvatarBox()
	if m.imageMode != media.ModeKitty || !m.kittyStore.Ready(components.ProfileAvatarImageKey, cols) {
		return m, nil
	}
	id := m.kittyStore.IDFor(components.ProfileAvatarImageKey)
	m.kittyStore.Untransmit(components.ProfileAvatarImageKey)
	return m, func() tea.Msg { return tea.Raw(media.DeleteSeq(id))() }
}

// handleProfileRequest applies the overlay's own actions. The returned bool is
// false when msg is not one of them.
func (m RootModel) handleProfileRequest(msg tea.Msg) (RootModel, tea.Cmd, bool) {
	switch req := msg.(type) {
	case components.OpenProfileRequest:
		next, cmd := m.openProfile(req.UserID)
		return next, cmd, true

	case components.CloseProfileMsg:
		next, cmd := m.closeProfile()
		return next, cmd, true

	case profileLoadedMsg:
		next, cmd := m.handleProfileLoaded(req)
		return next, cmd, true

	case avatarReadyMsg:
		next, cmd := m.handleAvatarReady(req)
		return next, cmd, true

	case components.ProfileOpenChatRequest:
		m, closeCmd := m.closeProfile()
		// The peer is carried for the person with no dialog: the owner holds no
		// chat for them, so nothing but a send can address it, and the chat
		// opens empty and composable. This is the same path a search hit takes.
		//
		// TRANSITIONAL (#198): when commands become owner API members addressed
		// by chat id, the peer goes.
		userID := req.UserID
		var peer domain.Peer
		title := ""
		if m.st != nil {
			if chat, found := m.st.GetChat(userID); found {
				title = chat.Title
			}
		}
		if title == "" && m.owner != nil {
			if user, ok := m.owner.KnownUser(userID); ok {
				title = user.DisplayName()
				peer = domain.Peer{ID: userID, Type: domain.PeerUser}
			}
		}
		return m, tea.Batch(closeCmd, func() tea.Msg {
			return screens.OpenChatMsg{ChatID: userID, Title: title, Peer: peer}
		}), true

	case components.ProfileMuteRequest:
		if m.owner == nil {
			return m, nil, true
		}
		// The same owner command the chat context menu issues, so the two
		// cannot disagree about what muting means.
		ctx, owner, userID, muted := m.ctx, m.owner, req.UserID, req.Muted
		return m, func() tea.Msg {
			if err := owner.SetMuted(ctx, userID, muted); err != nil {
				return errStatus("mute", err)
			}
			return nil
		}, true

	case components.ProfileCopyUsernameRequest:
		next, closeCmd := m.closeProfile()
		return next, tea.Batch(closeCmd, copyUsernameCmd(req.Username)), true
	}
	return m, nil, false
}

// copyUsernameCmd puts the handle on the clipboard and names what it copied: in
// an overlay holding a name, a handle and a phone, "Copied" alone does not say
// which of them went.
func copyUsernameCmd(handle string) tea.Cmd {
	return func() tea.Msg {
		return usernameCopiedMsg{handle: handle, ok: clipboardWrite(handle) == nil}
	}
}

type usernameCopiedMsg struct {
	handle string
	ok     bool
}
