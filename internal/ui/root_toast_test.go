package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// mainScreenModel builds a sized main-screen RootModel for toast tests.
func mainScreenModel() RootModel {
	m := NewRootModel(nil, nil, 50, false).WithScreen(ScreenMain)
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
	if !rm2.toasts.Empty() {
		t.Fatal("ctrl+x should dismiss the top toast")
	}
}

func TestMouseClick_ToastActionEmitsMsg(t *testing.T) {
	rm := mainScreenModel()
	// A toast carrying a clickable action.
	rm.toasts.Add(components.ToastError, "click me",
		components.ToastAction{Label: "close", Key: "x", Msg: ClearStatusErrMsg{Serial: 0}})

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

func notifyModel(t *testing.T, chat store.Chat) RootModel {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(chat)
	m := NewRootModel(nil, st, 50, false).WithScreen(ScreenMain)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return model.(RootModel)
}

func newMessageEvent(chatID int64, text string, out bool) store.Event {
	return store.Event{
		Kind: store.EventNewMessage,
		Message: store.Message{
			ID: 1000, ChatID: chatID, Text: text, IsOut: out, Date: time.Now(),
		},
	}
}

func TestInAppNotify_InactiveChat_ShowsToast(t *testing.T) {
	m := notifyModel(t, store.Chat{ID: 7, Title: "Alice"})
	m.currentChatID = 0 // no active chat
	model, _ := m.Update(newMessageEvent(7, "hey there", false))
	rm := model.(RootModel)
	if rm.toasts.Empty() {
		t.Fatal("expected an in-app notify toast")
	}
	view := rm.View().Content
	if !strings.Contains(view, "Alice") || !strings.Contains(view, "hey there") {
		t.Fatalf("toast missing title/preview:\n%s", view)
	}
}

func TestInAppNotify_ClickOpensChat(t *testing.T) {
	m := notifyModel(t, store.Chat{ID: 7, Title: "Alice"})
	m.currentChatID = 0
	model, _ := m.Update(newMessageEvent(7, "hi", false))
	rm := model.(RootModel)

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
	if open.Chat.ID != 7 {
		t.Fatalf("open chat ID = %d, want 7", open.Chat.ID)
	}
}

func TestInAppNotify_ActiveChat_NoToast(t *testing.T) {
	m := notifyModel(t, store.Chat{ID: 7, Title: "Alice"})
	m.currentChatID = 7 // this chat is open
	model, _ := m.Update(newMessageEvent(7, "hey", false))
	if !model.(RootModel).toasts.Empty() {
		t.Fatal("active chat must not notify")
	}
}

func TestInAppNotify_Outgoing_NoToast(t *testing.T) {
	m := notifyModel(t, store.Chat{ID: 7, Title: "Alice"})
	m.currentChatID = 0
	model, _ := m.Update(newMessageEvent(7, "sent by me", true))
	if !model.(RootModel).toasts.Empty() {
		t.Fatal("outgoing message must not notify")
	}
}

func TestInAppNotify_MutedChat_NoToast(t *testing.T) {
	m := notifyModel(t, store.Chat{ID: 7, Title: "Alice", IsMuted: true})
	m.currentChatID = 0
	model, _ := m.Update(newMessageEvent(7, "hey", false))
	if !model.(RootModel).toasts.Empty() {
		t.Fatal("muted chat must not notify")
	}
}

func TestInAppNotify_StaleMessage_NoToast(t *testing.T) {
	m := notifyModel(t, store.Chat{ID: 7, Title: "Alice"})
	m.currentChatID = 0
	evt := store.Event{
		Kind: store.EventNewMessage,
		Message: store.Message{
			ID: 1001, ChatID: 7, Text: "old", IsOut: false,
			Date: time.Now().Add(-store.NotifyFreshnessWindow - time.Second),
		},
	}
	model, _ := m.Update(evt)
	if !model.(RootModel).toasts.Empty() {
		t.Fatal("stale catch-up message must not notify")
	}
}

func TestInAppNotify_PreviewOff_HidesText(t *testing.T) {
	m := notifyModel(t, store.Chat{ID: 7, Title: "Alice"})
	m.currentChatID = 0
	m.cfg = &config.Config{}
	m.cfg.UI.NotificationPreview = false
	model, _ := m.Update(newMessageEvent(7, "secret text", false))
	view := model.(RootModel).View().Content
	if strings.Contains(view, "secret text") {
		t.Fatalf("preview-off must hide message text:\n%s", view)
	}
	if !strings.Contains(view, "Alice") {
		t.Fatalf("title should still show:\n%s", view)
	}
}

func TestParseToastZone(t *testing.T) {
	if parseToastZone("top-right") != components.ZoneTopRight {
		t.Fatal("top-right")
	}
	if parseToastZone("bottom-left") != components.ZoneBottomLeft {
		t.Fatal("bottom-left")
	}
	if parseToastZone("garbage") != components.ZoneBottomRight {
		t.Fatal("unknown must default to bottom-right")
	}
}
