package ui

import (
	tea "charm.land/bubbletea/v2"
)

// logoShouldTick reports whether an animated logo is currently on screen: the
// login splash, or the idle logo in the chat pane on the main screen. The 80ms
// logo loop runs only while one of these is visible (issue #147).
func (m *RootModel) logoShouldTick() bool {
	if m.screen == ScreenLogin {
		return true
	}
	return m.screen == ScreenMain && m.chat.ShowingLogo()
}

// spinnerShouldTick reports whether any animated spinner is currently active on
// the main screen: the chat-list loading spinner, the chat loading spinner, a
// download indicator, a GIF being fetched, or a video modal still loading its
// first frame. Driving the 150ms spinner loop only while one of these is live
// keeps the app asleep at idle (issue #147).
func (m *RootModel) spinnerShouldTick() bool {
	if m.screen != ScreenMain {
		return false
	}
	switch {
	case m.chatList.IsLoadingChats():
		return true
	case m.chat.IsLoading():
		return true
	case m.statusBar.DownloadActive():
		return true
	case m.gifActiveID != 0 && len(m.gifFrames[m.gifActiveID]) == 0:
		return true
	case m.videoPlayer != nil && m.videoPlayer.frame == nil:
		return true
	case m.photoViewer != nil && m.photoViewer.img == nil:
		return true
	}
	return false
}

// toastShouldTick reports whether any toast is mid-slide (entering or leaving).
func (m *RootModel) toastShouldTick() bool {
	return m.toasts.Animating()
}

// ensureAnimationTicks re-arms a tick loop whose content has become
// visible/active while the loop was asleep. It runs after every event (in
// Update), so an idle→active transition restarts the loop without each call
// site needing to know about the tickers. A loop already running (its flag set)
// is left alone, so no duplicate ticks are scheduled.
func (m *RootModel) ensureAnimationTicks() tea.Cmd {
	var cmds []tea.Cmd
	if !m.logoTicking && m.logoShouldTick() {
		m.logoTicking = true
		cmds = append(cmds, logoTickCmd())
	}
	if !m.spinnerTicking && m.spinnerShouldTick() {
		m.spinnerTicking = true
		cmds = append(cmds, spinnerTickCmd())
	}
	if !m.toastAnimTicking && m.toastShouldTick() {
		m.toastAnimTicking = true
		cmds = append(cmds, toastAnimTickCmd())
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}
