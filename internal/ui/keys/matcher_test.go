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
		"j":      keys.ActionDown,
		"k":      keys.ActionUp,
		"down":   keys.ActionDown,
		"up":     keys.ActionUp,
		"G":      keys.ActionGoBottom,
		"ctrl+d": keys.ActionScrollHalfDown,
		"ctrl+u": keys.ActionScrollHalfUp,
		"i":      keys.ActionInsert,
		"a":      keys.ActionInsert,
		"esc":    keys.ActionNormal,
		"enter":  keys.ActionConfirm,
		"/":      keys.ActionSearch,
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

func TestMatcher_DefaultChatListGG(t *testing.T) {
	m := keys.NewMatcher(keys.DefaultKeyMap())
	_, res := m.Resolve(keys.ContextChatList, "g")
	assert.Equal(t, keys.MatchPending, res)
	action, res := m.Resolve(keys.ContextChatList, "g")
	assert.Equal(t, keys.ActionGoTop, action)
	assert.Equal(t, keys.MatchAction, res)
}
