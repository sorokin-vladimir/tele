package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// mainScreenModel builds a sized main-screen RootModel for toast tests.
func mainScreenModel() RootModel {
	m := NewRootModel(nil, 50, false).WithScreen(ScreenMain)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return model.(RootModel)
}

// drainClearSerial extracts the ClearStatusErrMsg serial from a scheduled cmd.
func drainClearSerial(t *testing.T, cmd tea.Cmd) int {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a tick command")
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, c := range b {
			if cs, ok := c().(ClearStatusErrMsg); ok {
				return cs.Serial
			}
		}
		t.Fatal("no ClearStatusErrMsg in batch")
	}
	if cs, ok := msg.(ClearStatusErrMsg); ok {
		return cs.Serial
	}
	t.Fatalf("unexpected msg %T", msg)
	return 0
}

func TestStatusErr_RendersInToastNotStatusBar(t *testing.T) {
	m := mainScreenModel()
	model, _ := m.Update(StatusErrMsg{Text: "connection lost", Sev: components.SeverityError})
	rm := model.(RootModel)
	if rm.toasts.Empty() {
		t.Fatal("StatusErrMsg should add a toast")
	}
	rm.SettleToastsForTest()
	view := rm.View().Content
	if !strings.Contains(view, "connection lost") {
		t.Fatalf("toast text not in view:\n%s", view)
	}
}

func TestClearStatusErr_DismissesToast(t *testing.T) {
	m := mainScreenModel()
	model, cmd := m.Update(StatusErrMsg{Text: "boom", Sev: components.SeverityError})
	rm := model.(RootModel)
	serial := drainClearSerial(t, cmd)
	model2, _ := rm.Update(ClearStatusErrMsg{Serial: serial})
	rm2 := model2.(RootModel)
	rm2.SettleToastsForTest()
	if !rm2.toasts.Empty() {
		t.Fatal("ClearStatusErrMsg should dismiss the toast")
	}
}

func TestDismissToastAction_ClosesTopToast(t *testing.T) {
	m := mainScreenModel()
	model, _ := m.Update(StatusErrMsg{Text: "boom", Sev: components.SeverityError})
	rm := model.(RootModel)
	model2, _ := rm.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	rm2 := model2.(RootModel)
	rm2.SettleToastsForTest()
	if !rm2.toasts.Empty() {
		t.Fatal("ctrl+x should dismiss the top toast")
	}
}

func TestMouseClick_ToastActionEmitsMsg(t *testing.T) {
	rm := mainScreenModel()
	// A toast carrying a clickable action.
	rm.toasts.Add(components.ToastError, "click me",
		components.ToastAction{Label: "close", Key: "x", Msg: ClearStatusErrMsg{Serial: 0}})
	rm.SettleToastsForTest()

	rects := rm.toasts.HitTestRects()
	if len(rects) == 0 {
		t.Fatal("expected an action region")
	}
	r := rects[0].Rect
	cx, cy := r.Left+r.Width/2, r.Top+r.Height/2
	_, cmd := rm.handleMouseClick(tea.Mouse{X: cx, Y: cy, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("clicking an action should return a command")
	}
}

func TestChatLoadErr_ToastHasRetryAction(t *testing.T) {
	m := mainScreenModel()
	m.currentChatID = 42
	model, _ := m.Update(chatLoadErrMsg{chatID: 42, text: "load failed"})
	rm := model.(RootModel)
	rm.SettleToastsForTest()
	found := false
	for _, r := range rm.toasts.HitTestRects() {
		if _, ok := r.Msg.(retryChatLoadMsg); ok {
			found = true
		}
	}
	if !found {
		t.Fatal("chat-load error toast must carry a retry action")
	}
}

func notifyModel(t *testing.T, chat domain.Chat) RootModel {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(chat)
	m := newRootInternal(st, 50).WithScreen(ScreenMain)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return model.(RootModel)
}

// A toast renders a decision the owner already made: the model is handed a
// Notification and does not judge it (#192).
func TestNotification_ShowsToast(t *testing.T) {
	m := notifyModel(t, domain.Chat{ID: 7, Title: "Alice"})
	model, _ := m.Update(core.Notification{ChatID: 7, Title: "Alice", Body: "hey there"})
	rm := model.(RootModel)
	if rm.toasts.Empty() {
		t.Fatal("expected an in-app notify toast")
	}
	rm.SettleToastsForTest()
	view := rm.View().Content
	if !strings.Contains(view, "Alice") || !strings.Contains(view, "hey there") {
		t.Fatalf("toast missing title/body:\n%s", view)
	}
}

// The row flash and the toast are separate decisions: an Incoming says only that
// a chat moved, and must not interrupt anybody on its own.
func TestIncoming_FlashesRowWithoutToast(t *testing.T) {
	m := notifyModel(t, domain.Chat{ID: 7, Title: "Alice"})
	model, _ := m.Update(core.Incoming{ChatID: 7})
	rm := model.(RootModel)
	rm.SettleToastsForTest()
	if !rm.toasts.Empty() {
		t.Fatal("an Incoming alone must not raise a toast")
	}
}

func TestNotification_ClickOpensChat(t *testing.T) {
	m := notifyModel(t, domain.Chat{ID: 7, Title: "Alice"})
	model, _ := m.Update(core.Notification{ChatID: 7, Title: "Alice", Body: "hi"})
	rm := model.(RootModel)
	rm.SettleToastsForTest()

	// The notify toast is a whole-box click target emitting notifyOpenMsg.
	var click tea.Msg
	for _, r := range rm.toasts.HitTestRects() {
		if _, ok := r.Msg.(notifyOpenMsg); ok {
			click = r.Msg
		}
	}
	if click == nil {
		t.Fatal("notify toast must be clickable to open its chat")
	}

	// Handling it dismisses the toast and emits OpenChatMsg for the chat.
	model2, cmd := rm.Update(click)
	rm2 := model2.(RootModel)
	rm2.SettleToastsForTest()
	if !rm2.toasts.Empty() {
		t.Fatal("clicking should dismiss the notify toast")
	}
	if cmd == nil {
		t.Fatal("clicking should emit an open-chat command")
	}
	open, ok := cmd().(screens.OpenChatMsg)
	if !ok {
		t.Fatalf("expected OpenChatMsg, got %T", cmd())
	}
	if open.ChatID != 7 {
		t.Fatalf("open chat ID = %d, want 7", open.ChatID)
	}
}

// The cases this file used to own — muted, stale, preview-off, the open chat,
// an outgoing message — are decisions, not rendering. They live in
// internal/core/notify_test.go, against the real policy rather than a copy of
// it (#192).

// The two corners toasts are sent to. The bottom-left corner is not one of
// them: it is held for the demo mode's key-press visualiser (#83), so a toast
// there would be drawn by nobody. The settings registry declares the same two,
// which is what stops a config from naming a third.
func TestParseToastZone(t *testing.T) {
	if parseToastZone("top-right") != components.ZoneTopRight {
		t.Fatal("top-right")
	}
	if parseToastZone("bottom-right") != components.ZoneBottomRight {
		t.Fatal("bottom-right")
	}
	if parseToastZone("garbage") != components.ZoneBottomRight {
		t.Fatal("unknown must default to bottom-right")
	}
}
