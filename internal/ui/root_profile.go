package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
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
	ctx, owner := m.ctx, m.owner
	return m, func() tea.Msg {
		full, err := owner.GetUser(ctx, userID)
		if err != nil {
			// A profile that could not be completed stays as it opened. The
			// name was already on screen; saying so again would be noise.
			return nil
		}
		return profileLoadedMsg{userID: userID, user: full}
	}
}

// handleProfileLoaded completes an open overlay, ignoring an answer for a
// profile that has since been closed or replaced.
func (m RootModel) handleProfileLoaded(msg profileLoadedMsg) (RootModel, tea.Cmd) {
	if m.profile == nil || m.profile.UserID() != msg.userID {
		return m, nil
	}
	m.profile.SetUser(msg.user)
	return m, nil
}

// handleProfileRequest applies the overlay's own actions. The returned bool is
// false when msg is not one of them.
func (m RootModel) handleProfileRequest(msg tea.Msg) (RootModel, tea.Cmd, bool) {
	switch req := msg.(type) {
	case components.OpenProfileRequest:
		next, cmd := m.openProfile(req.UserID)
		return next, cmd, true

	case components.CloseProfileMsg:
		m.profile = nil
		return m, nil, true

	case profileLoadedMsg:
		next, cmd := m.handleProfileLoaded(req)
		return next, cmd, true

	case components.ProfileOpenChatRequest:
		m.profile = nil
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
		return m, func() tea.Msg {
			return screens.OpenChatMsg{ChatID: userID, Title: title, Peer: peer}
		}, true

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
		m.profile = nil
		return m, copyUsernameCmd(req.Username), true
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
