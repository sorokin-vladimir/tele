package keys_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
)

func testMatcherMap() keys.KeyMap {
	return keys.KeyMap{
		keys.ContextChat: {
			"j":   keys.ActionDown,
			"g g": keys.ActionGoTop,
		},
		keys.ContextGlobal: {
			"ctrl+c": keys.ActionQuit,
		},
	}
}

// The log out chord shares its first key with "g g" in the chat and the chat
// list, and has to work from both, while "g g" still goes to the top (#297).
func TestMatcher_LogOutChordWorksWhereGGIsBoundToo(t *testing.T) {
	for _, ctx := range []keys.Context{keys.ContextChat, keys.ContextChatList, keys.ContextFolders} {
		m := keys.NewMatcher(keys.DefaultKeyMap())

		_, res := m.Resolve(ctx, "g")
		assert.Equal(t, keys.MatchPending, res, ctx)
		action, _ := m.Resolve(ctx, "L")
		assert.Equal(t, keys.ActionLogOut, action, ctx)
	}

	m := keys.NewMatcher(keys.DefaultKeyMap())
	m.Resolve(keys.ContextChat, "g")
	action, _ := m.Resolve(keys.ContextChat, "g")
	assert.Equal(t, keys.ActionGoTop, action)
}

func TestMatcher_SingleKey(t *testing.T) {
	m := keys.NewMatcher(testMatcherMap())
	action, res := m.Resolve(keys.ContextChat, "j")
	assert.Equal(t, keys.ActionDown, action)
	assert.Equal(t, keys.MatchAction, res)
}

func TestMatcher_Chord(t *testing.T) {
	m := keys.NewMatcher(testMatcherMap())

	action, res := m.Resolve(keys.ContextChat, "g")
	assert.Equal(t, keys.ActionNone, action)
	assert.Equal(t, keys.MatchPending, res)

	action, res = m.Resolve(keys.ContextChat, "g")
	assert.Equal(t, keys.ActionGoTop, action)
	assert.Equal(t, keys.MatchAction, res)
}

func TestMatcher_DeadPrefixReprocessesKey(t *testing.T) {
	m := keys.NewMatcher(testMatcherMap())

	_, res := m.Resolve(keys.ContextChat, "g")
	assert.Equal(t, keys.MatchPending, res)

	// "g" then "j": not a chord; the "j" must still resolve on its own.
	action, res := m.Resolve(keys.ContextChat, "j")
	assert.Equal(t, keys.ActionDown, action)
	assert.Equal(t, keys.MatchAction, res)
}

func TestMatcher_UnknownKey(t *testing.T) {
	m := keys.NewMatcher(testMatcherMap())
	action, res := m.Resolve(keys.ContextChat, "x")
	assert.Equal(t, keys.ActionNone, action)
	assert.Equal(t, keys.MatchNone, res)
}

func TestMatcher_GlobalFallback(t *testing.T) {
	m := keys.NewMatcher(testMatcherMap())
	action, res := m.Resolve(keys.ContextChat, "ctrl+c")
	assert.Equal(t, keys.ActionQuit, action)
	assert.Equal(t, keys.MatchAction, res)
}

func TestMatcher_Reset(t *testing.T) {
	m := keys.NewMatcher(testMatcherMap())

	_, res := m.Resolve(keys.ContextChat, "g")
	assert.Equal(t, keys.MatchPending, res)

	m.Reset()

	action, res := m.Resolve(keys.ContextChat, "j")
	assert.Equal(t, keys.ActionDown, action)
	assert.Equal(t, keys.MatchAction, res)
}

func TestMatcher_DefaultChatBindings(t *testing.T) {
	m := keys.NewMatcher(keys.DefaultKeyMap())

	cases := map[string]keys.Action{
		"j":      keys.ActionCursorDown,
		"k":      keys.ActionCursorUp,
		"down":   keys.ActionDown,
		"up":     keys.ActionUp,
		"ctrl+j": keys.ActionDown,
		"ctrl+k": keys.ActionUp,
		"G":      keys.ActionGoBottom,
		"ctrl+d": keys.ActionScrollHalfDown,
		"ctrl+u": keys.ActionScrollHalfUp,
		"i":      keys.ActionInsert,
		"a":      keys.ActionInsert,
		"esc":    keys.ActionNormal,
		"enter":  keys.ActionConfirm,
		"/":      keys.ActionMessageSearch,
		"space":  keys.ActionOpenContextMenu,
		"r":      keys.ActionReply,
		"e":      keys.ActionEdit,
		"o":      keys.ActionOpenInViewer,
	}
	for key, want := range cases {
		m.Reset()
		action, res := m.Resolve(keys.ContextChat, key)
		assert.Equal(t, keys.MatchAction, res, "key %q", key)
		assert.Equal(t, want, action, "key %q", key)
	}
}

func TestMatcher_DefaultChatGG(t *testing.T) {
	m := keys.NewMatcher(keys.DefaultKeyMap())
	_, res := m.Resolve(keys.ContextChat, "g")
	assert.Equal(t, keys.MatchPending, res)
	action, res := m.Resolve(keys.ContextChat, "g")
	assert.Equal(t, keys.ActionGoTop, action)
	assert.Equal(t, keys.MatchAction, res)
}

func TestMatcher_ContextOwns(t *testing.T) {
	m := keys.NewMatcher(testMatcherMap())
	// Explicit terminal binding in the context.
	assert.True(t, m.ContextOwns(keys.ContextChat, "j"))
	// First token of a chord is a live prefix the context owns.
	assert.True(t, m.ContextOwns(keys.ContextChat, "g"))
	// A key bound only in global is NOT owned by the focused context.
	assert.False(t, m.ContextOwns(keys.ContextChat, "ctrl+c"))
	// Unbound key.
	assert.False(t, m.ContextOwns(keys.ContextChat, "x"))
}

func TestMatcher_DefaultChatListGG(t *testing.T) {
	m := keys.NewMatcher(keys.DefaultKeyMap())
	_, res := m.Resolve(keys.ContextChatList, "g")
	assert.Equal(t, keys.MatchPending, res)
	action, res := m.Resolve(keys.ContextChatList, "g")
	assert.Equal(t, keys.ActionGoTop, action)
	assert.Equal(t, keys.MatchAction, res)
}
