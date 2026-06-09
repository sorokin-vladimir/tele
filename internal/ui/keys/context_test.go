package keys_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
)

func TestKeyMap_Resolve_ChatList(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionDown, km.Resolve(keys.ContextChatList, "j"))
}

func TestKeyMap_Resolve_GlobalFallback(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionFocusChatList, km.Resolve(keys.ContextChatList, "1"))
	assert.Equal(t, keys.ActionFocusChat, km.Resolve(keys.ContextChatList, "2"))
	assert.Equal(t, keys.ActionFocusPrev, km.Resolve(keys.ContextChatList, "left"))
	assert.Equal(t, keys.ActionFocusNext, km.Resolve(keys.ContextChatList, "right"))
}

func TestKeyMap_Resolve_Unknown(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionNone, km.Resolve(keys.ContextChatList, "F9"))
}

func TestKeyFor_SingleBinding_ReturnsIt(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, "enter", km.KeyFor(keys.ContextChatList, keys.ActionConfirm))
}

func TestKeyFor_MultipleBindings_ReturnsShortest(t *testing.T) {
	km := keys.DefaultKeyMap()
	// "j" (len 1) vs "down" (len 4) → "j"
	assert.Equal(t, "j", km.KeyFor(keys.ContextChatList, keys.ActionDown))
	// "k" (len 1) vs "up" (len 2) → "k"
	assert.Equal(t, "k", km.KeyFor(keys.ContextChatList, keys.ActionUp))
	// "q" (len 1) vs "ctrl+c", "ctrl+q" (len 6) → "q"
	assert.Equal(t, "q", km.KeyFor(keys.ContextGlobal, keys.ActionQuit))
	// "esc" (len 3) vs "space" (len 5) in context menu → "esc"
	assert.Equal(t, "esc", km.KeyFor(keys.ContextContextMenu, keys.ActionCancel))
}

func TestKeyFor_SameLengthBindings_ReturnsAlphabeticallyFirst(t *testing.T) {
	km := keys.KeyMap{
		keys.ContextGlobal: {
			"b": keys.ActionDown,
			"a": keys.ActionDown,
		},
	}
	assert.Equal(t, "a", km.KeyFor(keys.ContextGlobal, keys.ActionDown))
}

func TestKeyFor_UnknownAction_ReturnsEmpty(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, "", km.KeyFor(keys.ContextChatList, keys.ActionReply))
}

func TestKeyFor_UnknownContext_ReturnsEmpty(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, "", km.KeyFor("nonexistent", keys.ActionDown))
}

func TestKeyFor_NilMap_ReturnsEmpty(t *testing.T) {
	var km keys.KeyMap
	assert.Equal(t, "", km.KeyFor(keys.ContextGlobal, keys.ActionDown))
}

func TestKeyMap_ChatMenuBindings(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionMute, km.Resolve(keys.ContextChatMenu, "m"))
	assert.Equal(t, keys.ActionAddToFolder, km.Resolve(keys.ContextChatMenu, "f"))
	assert.Equal(t, keys.ActionConfirm, km.Resolve(keys.ContextFolderSubMenu, "enter"))
	assert.Equal(t, keys.ActionOpenContextMenu, km.Resolve(keys.ContextChatList, "space"))
}
