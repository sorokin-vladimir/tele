package ui_test

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui"
)

// View is the whole render: every frame goes through it, and it is where the
// canvas work of #214 lands — a background baked into every style and every
// padded cell. This benchmark exists so "did it get slower" is a number rather
// than an impression. It was written before that work started, so the figures it
// prints on the commit that introduced it are the baseline.
//
// The sizes are the ones that behave differently rather than a sweep: 80x24 is
// the floor a terminal is ever opened at, 120x40 is an ordinary window, 200x60
// is wide enough that the padding the panes emit dominates the text they hold.

// benchChats is how many rows the chat list holds. Only a window of them is
// rendered, but the list has to be big enough that the window is a window.
const benchChats = 200

// benchMessages is the backlog in the open chat. The message list renders from
// the bottom, so what matters is that there are more than fit.
const benchMessages = 200

// newPopulatedRoot builds a main-screen model with a populated chat list and an
// open chat, sized to w x h. It is the state the app spends its time in, which
// makes it both what a benchmark should measure and what a canvas scan should
// look at.
func newPopulatedRoot(b testing.TB, w, h int) ui.RootModel {
	b.Helper()

	st := store.NewMemory()
	chats := make([]domain.Chat, 0, benchChats)
	for i := 1; i <= benchChats; i++ {
		c := domain.Chat{
			ID:          int64(i),
			Title:       fmt.Sprintf("Contact %d", i),
			Peer:        domain.Peer{ID: int64(i), Type: domain.PeerUser},
			UnreadCount: i % 7,
			IsMuted:     i%5 == 0,
			Online:      i%3 == 0,
		}
		st.SetChat(c)
		chats = append(chats, c)
	}

	msgs := make([]domain.Message, 0, benchMessages)
	for i := 1; i <= benchMessages; i++ {
		msgs = append(msgs, domain.Message{
			ID:     i,
			ChatID: 1,
			// Mixed lengths so wrapping and bubble sizing do real work, and
			// alternating direction so both bubble borders are exercised.
			Text:  fmt.Sprintf("message %d: the quick brown fox jumps over the lazy dog", i),
			IsOut: i%2 == 0,
			Date:  time.Unix(int64(i), 0),
		})
	}
	st.SetMessages(1, msgs)

	m := newRoot(st, benchMessages, false)
	m = m.WithScreen(ui.ScreenMain)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(ui.RootModel)
	m = toMain(b, m)
	setChatListWindow(m.ChatList(), chats)
	m = openChat(b, m, 1, "Contact 1")
	if _, cmd := applyHistory(b, m, st, 1); cmd != nil {
		cmd()
	}
	return m
}

func BenchmarkRootView(b *testing.B) {
	for _, size := range []struct{ w, h int }{
		{80, 24},
		{120, 40},
		{200, 60},
	} {
		b.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(b *testing.B) {
			m := newPopulatedRoot(b, size.w, size.h)
			// Rendering once outside the loop keeps a first-call cache fill out
			// of the first iteration's time.
			_ = m.View()

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = m.View().Content
			}
		})
	}
}

// The modal path is measured separately because it is the expensive one:
// dimBackground rewrites every line of the composed screen, and it is the one
// place a canvas has to survive an ANSI strip.
func BenchmarkRootView_HelpModal(b *testing.B) {
	m := newPopulatedRoot(b, 120, 40)
	next, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = next.(ui.RootModel)
	_ = m.View()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = m.View().Content
	}
}
