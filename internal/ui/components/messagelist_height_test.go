package components

import (
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/store"
)

// TestMsgHeightMatchesRenderMessage guards the invariant that positionAtBottom
// and ScrollDown rely on: the estimated msgHeight must equal the number of lines
// renderMessage actually produces. When it under-counts, positionAtBottom anchors
// too low and the freshly arrived tail message is clipped/unreachable (issue #115).
func TestMsgHeightMatchesRenderMessage(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		msgs []store.Message
		// target is the index in msgs of the message under test.
		target int
	}{
		{"plain short", []store.Message{{ID: 1, Text: "ok", Date: now}}, 0},
		{"plain outgoing", []store.Message{{ID: 1, Text: "ok", IsOut: true, Date: now}}, 0},
		{"multiline", []store.Message{{ID: 1, Text: "line one\nline two\nline three", Date: now}}, 0},
		{"empty text", []store.Message{{ID: 1, Text: "", Date: now}}, 0},
		{
			name: "reply orig present",
			msgs: []store.Message{
				{ID: 1, Text: "original", Date: now},
				{ID: 2, Text: "a reply", ReplyToMsgID: 1, Date: now},
			},
			target: 1,
		},
		{
			name:   "reply orig absent",
			msgs:   []store.Message{{ID: 2, Text: "a reply", ReplyToMsgID: 999, Date: now}},
			target: 0,
		},
		{
			name:   "forward no text",
			msgs:   []store.Message{{ID: 1, Date: now, Forward: &store.ForwardInfo{From: "Alice"}}},
			target: 0,
		},
		{
			name:   "forward with text",
			msgs:   []store.Message{{ID: 1, Text: "fwded note", Date: now, Forward: &store.ForwardInfo{From: "Alice"}}},
			target: 0,
		},
		{
			name:   "forward hidden sender",
			msgs:   []store.Message{{ID: 1, Text: "hi", Date: now, Forward: &store.ForwardInfo{From: ""}}},
			target: 0,
		},
		{
			name:   "edited short",
			msgs:   []store.Message{{ID: 1, Text: "ok", Date: now, EditDate: &now}},
			target: 0,
		},
		{
			name: "with reactions",
			msgs: []store.Message{{ID: 1, Text: "ok", Date: now, Reactions: []store.Reaction{{Emoji: "👍", Count: 2}}}},
			target: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ml := NewMessageList(50, 40)
			ml.SetMessages(tc.msgs)
			msg := tc.msgs[tc.target]
			est := ml.msgHeight(msg)
			got := len(ml.renderMessage(msg, false))
			if est != got {
				t.Errorf("height mismatch: msgHeight=%d, renderMessage produced %d lines", est, got)
			}
		})
	}
}
