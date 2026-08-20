package ui

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

func queuedEntry(ref, text string) domain.OutboxEntry {
	return domain.OutboxEntry{
		Ref: ref, ChatID: 1, State: domain.OutboxQueued,
		Message: &domain.OutboxMessage{Text: text},
	}
}

// A reset carries the whole contents, and the queue is part of it: a chat
// reopened with a send still pending must show it (#193).
func TestChatReset_SeatsTheOutbox(t *testing.T) {
	_, m := anchorTestModel()

	m, _ = m.handleChatDelta(&project.ChatDelta{
		Kind: project.ChatReset,
		Contents: project.ChatContents{
			ChatID:   1,
			Messages: mediaWindow(),
			Outbox:   []domain.OutboxEntry{queuedEntry("r1", "pending")},
		},
	})

	got := m.chat.Outbox()
	if len(got) != 1 || got[0].Ref != "r1" {
		t.Fatalf("the reset must seat the queue, got %+v", got)
	}
}

func TestChatOutboxDelta_ReplacesTheQueue(t *testing.T) {
	_, m := anchorTestModel()
	m, _ = m.handleChatDelta(&project.ChatDelta{
		Kind:   project.ChatOutbox,
		Outbox: []domain.OutboxEntry{queuedEntry("r1", "pending")},
	})

	// The send landed: the owner dropped the entry and published the new list.
	m, _ = m.handleChatDelta(&project.ChatDelta{Kind: project.ChatOutbox})

	if got := m.chat.Outbox(); len(got) != 0 {
		t.Fatalf("a delivered entry must leave the pane, got %+v", got)
	}
}

// One recompute yields both the append and the emptied queue, but DiffChat puts
// the window first and the client consumes one delta per tea.Msg — so it
// renders a frame in between. That frame used to hold the message and the entry
// that produced it: the same text twice, the pane a bubble taller, everything
// above it shifted and shifted back. The blink of #226.
func TestChatDelta_TheSendSwapsWithoutADoubledFrame(t *testing.T) {
	const text = "СООБЩЕНИЕ-РОВНО-ОДИН-РАЗ"
	older := domain.Message{ID: 7, ChatID: 1, Date: time.Unix(1, 0), Text: "раньше"}
	sent := domain.Message{ID: 10, ChatID: 1, Date: time.Unix(2, 0), IsOut: true, Text: text}
	entry := domain.OutboxEntry{
		Ref: "r1", ChatID: 1, State: domain.OutboxSending,
		Message: &domain.OutboxMessage{Text: text}, CreatedAt: time.Unix(2, 0),
		SentMsgIDs: []int{10},
	}

	_, m := anchorTestModel()
	m.chat.SetSize(60, 20)
	m, _ = m.handleChatDelta(&project.ChatDelta{
		Kind: project.ChatReset,
		Contents: project.ChatContents{
			ChatID: 1, Messages: []domain.Message{older}, Outbox: []domain.OutboxEntry{entry},
		},
	})
	queued := m.chat.View()
	if n := strings.Count(xansi.Strip(queued), text); n != 1 {
		t.Fatalf("the queued bubble must be the one thing showing the send, drawn %d times", n)
	}

	// Frame one: the message is in the window, the queue is untouched.
	m, _ = m.handleChatDelta(&project.ChatDelta{Kind: project.ChatAppend, Message: sent})
	landed := m.chat.View()
	if n := strings.Count(xansi.Strip(landed), text); n != 1 {
		t.Fatalf("the send is drawn %d times in the frame between the deltas", n)
	}

	// Frame two: the owner publishes the queue without it. Nothing may move.
	m, _ = m.handleChatDelta(&project.ChatDelta{Kind: project.ChatOutbox})
	if cleared := m.chat.View(); cleared != landed {
		t.Fatalf("clearing the entry redrew the pane:\n%s\n---\n%s", landed, cleared)
	}
}
