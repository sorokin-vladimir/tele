package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func tracedStore(t *testing.T) (*store.SQLiteStore, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.DebugLevel)
	s, err := store.NewSQLite(":memory:", zap.New(core))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s, logs
}

func reactionTrace(logs *observer.ObservedLogs) []observer.LoggedEntry {
	return logs.FilterMessage("reaction: store write").All()
}

var liked = []domain.Reaction{{Emoji: "👍", Count: 1, IsChosen: true}}

// A reaction that vanishes without an error (#248) was overwritten by some
// write, and the trace has to say which one did it. A set is written by an
// edit, a reactions update, our optimistic write and its rollback alike, so
// the caller names the source rather than the store method.
func TestSQLite_ReactionTrace_UpdateMessageReactions(t *testing.T) {
	s, logs := tracedStore(t)
	s.AppendMessage(domain.Message{ID: 1, ChatID: 5})

	s.UpdateMessageReactions(5, 1, liked, "optimistic")
	s.UpdateMessageReactions(5, 1, liked, "reactions update")

	entries := reactionTrace(logs)
	require.Len(t, entries, 1, "a write that changes nothing is not worth a line")
	assert.Equal(t, map[string]any{
		"chat_id": int64(5),
		"msg_id":  int64(1),
		"via":     "optimistic",
		"was":     "[]",
		"now":     "[👍:1:me]",
	}, entries[0].ContextMap())
}

func TestSQLite_ReactionTrace_WholeMessageWrites(t *testing.T) {
	s, logs := tracedStore(t)
	s.AppendMessage(domain.Message{ID: 1, ChatID: 5, Reactions: liked})
	s.AppendMessage(domain.Message{ID: 2, ChatID: 5})

	s.AppendMessage(domain.Message{ID: 1, ChatID: 5})
	s.ReplaceMessage(5, domain.Message{ID: 1, ChatID: 5, Reactions: liked})
	s.MergeOlder(domain.HistoryKey{ChatID: 5}, 0, []domain.Message{{ID: 1, ChatID: 5}, {ID: 2, ChatID: 5}})
	s.SetMessages(5, []domain.Message{{ID: 1, ChatID: 5, Reactions: liked}})

	var vias []string
	for _, e := range reactionTrace(logs) {
		vias = append(vias, e.ContextMap()["via"].(string))
	}
	assert.Equal(t, []string{"AppendMessage", "ReplaceMessage", "MergeOlder", "SetMessages"}, vias,
		"only the message whose reactions moved is reported, once per write")
}

func TestSQLite_ReactionTrace_SilentWithoutDebug(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	s, err := store.NewSQLite(":memory:", zap.New(core))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	s.AppendMessage(domain.Message{ID: 1, ChatID: 5})

	s.UpdateMessageReactions(5, 1, liked, "optimistic")

	assert.Empty(t, reactionTrace(logs))
}
