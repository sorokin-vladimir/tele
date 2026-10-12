package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// Logging out from tele (#297). A log out removes what cannot be had back from
// Telegram - the local history, the cached media, the messages still waiting
// to be sent - so it is confirmed first, with what goes and what stays named.
// Once it is done the client has nothing more to do: the host ends the account
// and puts the next one's login on screen in place of this model.

// logOutStep is where the confirmation has got to.
type logOutStep int

const (
	// logOutAsking waits for the person to confirm.
	logOutAsking logOutStep = iota
	// logOutWorking waits for the log out to finish.
	logOutWorking
	// logOutUnreached waits for the person to decide whether to log out here
	// although Telegram could not be told.
	logOutUnreached
)

// logOutDialog is the open confirmation.
type logOutDialog struct {
	preview core.LogOutPreview
	step    logOutStep
	// cause is why Telegram could not be told, for logOutUnreached.
	cause string
}

// logOutPreviewMsg brings what a log out would take, to confirm.
type logOutPreviewMsg struct{ preview core.LogOutPreview }

// logOutDoneMsg is how a log out went. A success says nothing more: the host
// takes over from there.
type logOutDoneMsg struct{ err error }

// askLogOut asks the owner what logging out would take, which opens the
// confirmation.
func (m RootModel) askLogOut() (RootModel, tea.Cmd) {
	if m.owner == nil || m.logOut != nil {
		return m, nil
	}
	owner := m.owner
	return m, func() tea.Msg { return logOutPreviewMsg{preview: owner.LogOutPreview()} }
}

func (m RootModel) handleLogOutPreview(msg logOutPreviewMsg) (RootModel, tea.Cmd) {
	m.logOut = &logOutDialog{preview: msg.preview}
	return m, nil
}

// handleLogOutKey answers the open confirmation. It owns every key while it is
// open, so nothing reaches the chat behind it.
func (m RootModel) handleLogOutKey(msg tea.KeyPressMsg) (RootModel, tea.Cmd, bool) {
	if m.logOut == nil {
		return m, nil, false
	}
	switch {
	case msg.Code == tea.KeyEscape:
		if m.logOut.step != logOutWorking {
			m.logOut = nil
		}
	// Read through the layout normalisation like every other key, so yes is
	// the key under y on a Russian layout too.
	case keys.NormalizeKey(msg.String()) == "y" && m.logOut.step != logOutWorking:
		// After Telegram could not be reached, yes means here only.
		tellTelegram := m.logOut.step == logOutAsking
		d := *m.logOut
		d.step = logOutWorking
		m.logOut = &d
		ctx, owner := m.ctx, m.owner
		return m, func() tea.Msg { return logOutDoneMsg{err: owner.LogOut(ctx, tellTelegram)} }, true
	}
	return m, nil, true
}

func (m RootModel) handleLogOutDone(msg logOutDoneMsg) (RootModel, tea.Cmd) {
	if m.logOut == nil || msg.err == nil {
		return m, nil
	}
	cause, _, _ := errText("Telegram could not be reached", msg.err)
	d := *m.logOut
	d.step, d.cause = logOutUnreached, cause
	m.logOut = &d
	return m, nil
}

// logOutView draws the open confirmation over content.
func (m RootModel) logOutView(content string) string {
	d := m.logOut
	maxW := min(max(m.width-10, 24), 72)
	var body, footer string
	switch d.step {
	case logOutAsking:
		removed := []string{
			"the session, here and at Telegram,",
			"this account's local history, cached media and avatars",
		}
		switch {
		case d.preview.Unsent == 1:
			removed[1] += ","
			removed = append(removed, "1 unsent message")
		case d.preview.Unsent > 1:
			removed[1] += ","
			removed = append(removed, strconv.Itoa(d.preview.Unsent)+" unsent messages")
		}
		removed[len(removed)-1] += "."
		body = "Log out " + d.preview.Name + "?\n\n" +
			"Removed: " + strings.Join(removed, "\n") + "\n" +
			"Kept: config, themes, log."
		footer = "y log out · Esc cancel"
	case logOutWorking:
		body = "Logging out " + d.preview.Name + "..."
	case logOutUnreached:
		body = d.cause + ".\n" +
			"The session stays in your devices list until it expires or is terminated there.\n\n" +
			"Log out here anyway?"
		footer = "y log out here · Esc cancel"
	}
	box := components.RenderDialog("Log out", body, footer, maxW)
	return overlayCenter(dimBackground(content), box, m.width, m.height)
}
