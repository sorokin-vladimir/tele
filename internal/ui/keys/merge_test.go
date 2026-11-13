package keys_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
)

func TestMergeOverrides_ReplaceRemovesDefaultKeys(t *testing.T) {
	base := keys.DefaultKeyMap()
	merged, warns := keys.MergeOverrides(base, map[string]map[string][]string{
		"chat": {"reply": {"R"}},
	})
	assert.Empty(t, warns)
	// New key resolves to reply.
	assert.Equal(t, keys.ActionReply, merged.Resolve(keys.ContextChat, "R"))
	// Default "r" no longer maps to reply (replace semantics).
	assert.NotEqual(t, keys.ActionReply, merged.Resolve(keys.ContextChat, "r"))
}

func TestMergeOverrides_UnmentionedActionsKeepDefaults(t *testing.T) {
	base := keys.DefaultKeyMap()
	merged, _ := keys.MergeOverrides(base, map[string]map[string][]string{
		"chat": {"reply": {"R"}},
	})
	assert.Equal(t, keys.ActionEdit, merged.Resolve(keys.ContextChat, "e"))
}

func TestMergeOverrides_DoesNotMutateBase(t *testing.T) {
	base := keys.DefaultKeyMap()
	_, _ = keys.MergeOverrides(base, map[string]map[string][]string{
		"chat": {"reply": {"R"}},
	})
	assert.Equal(t, keys.ActionReply, base.Resolve(keys.ContextChat, "r"))
}

func TestMergeOverrides_UnknownContextWarns(t *testing.T) {
	_, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"nope": {"reply": {"R"}},
	})
	assert.Len(t, warns, 1)
	assert.Contains(t, warns[0], "nope")
}

func TestMergeOverrides_UnknownActionWarns(t *testing.T) {
	_, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"chat": {"flyaway": {"R"}},
	})
	assert.Len(t, warns, 1)
	assert.Contains(t, warns[0], "flyaway")
}

func TestMergeOverrides_EmptyKeyWarns(t *testing.T) {
	_, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"chat": {"reply": {"  "}},
	})
	assert.Len(t, warns, 1)
	assert.Contains(t, warns[0], "reply")
}

func TestMergeOverrides_CollisionWarns(t *testing.T) {
	// Bind "e" (default edit) to down → collides with edit.
	_, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"chat": {"down": {"e"}},
	})
	assert.NotEmpty(t, warns)
	joined := ""
	for _, w := range warns {
		joined += w + "\n"
	}
	assert.Contains(t, joined, "e")
}

func TestMergeOverrides_ChordPrefixConflictWarns(t *testing.T) {
	// Bind insert to a single "g" while chat keeps the "g g" chord → "g g"
	// becomes unreachable.
	_, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"chat": {"insert": {"g"}},
	})
	joined := ""
	for _, w := range warns {
		joined += w + "\n"
	}
	assert.Contains(t, joined, "g g")
}

func TestMergeOverrides_GlobalActionsAreBindable(t *testing.T) {
	merged, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"global": {"quit": {"Q"}, "focus_chat": {"3"}},
	})
	assert.Empty(t, warns, "focus/quit actions must be bindable")
	assert.Equal(t, keys.ActionQuit, merged.Resolve(keys.ContextGlobal, "Q"))
	assert.Equal(t, keys.ActionFocusChat, merged.Resolve(keys.ContextGlobal, "3"))
}

// The searches are bindable, and so is search, the name chat_search had before
// tele searched messages: a config written then keeps its key (#349).
func TestMergeOverrides_SearchesAreBindable(t *testing.T) {
	merged, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"global":   {"chat_search": {"F"}, "message_search_all": {"g s"}},
		"chat":     {"message_search": {"S"}},
		"chatlist": {"search": {"s"}},
		"search":   {"toggle_scope": {"ctrl+t"}},
	})
	assert.Empty(t, warns)
	assert.Equal(t, keys.ActionChatSearch, merged.Resolve(keys.ContextGlobal, "F"))
	assert.Equal(t, keys.ActionMessageSearchAll, merged.Resolve(keys.ContextGlobal, "g s"))
	assert.Equal(t, keys.ActionMessageSearch, merged.Resolve(keys.ContextChat, "S"))
	assert.Equal(t, keys.ActionSearch, merged.Resolve(keys.ContextChatList, "s"))
	assert.Equal(t, keys.ActionToggleScope, merged.Resolve(keys.ContextSearch, "ctrl+t"))
}

func TestMergeOverrides_UnhandledLeftRightRejected(t *testing.T) {
	_, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"chat": {"left": {"H"}},
	})
	assert.Len(t, warns, 1)
	assert.Contains(t, warns[0], "left")
}

func TestMergeOverrides_Deterministic(t *testing.T) {
	ov := map[string]map[string][]string{
		"chat": {"down": {"x"}, "up": {"x"}},
	}
	_, first := keys.MergeOverrides(keys.DefaultKeyMap(), ov)
	for i := 0; i < 20; i++ {
		_, again := keys.MergeOverrides(keys.DefaultKeyMap(), ov)
		assert.Equal(t, first, again)
	}
}
