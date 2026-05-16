package keys_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

func TestKeyMap_Resolve_ChatList(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionDown, km.Resolve(keys.ContextChatList, "j"))
}

func TestKeyMap_Resolve_GlobalFallback(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionFocusLeft, km.Resolve(keys.ContextChatList, "1"))
	assert.Equal(t, keys.ActionFocusRight, km.Resolve(keys.ContextChatList, "2"))
	assert.Equal(t, keys.ActionFocusLeft, km.Resolve(keys.ContextChatList, "left"))
	assert.Equal(t, keys.ActionFocusRight, km.Resolve(keys.ContextChatList, "right"))
}

func TestKeyMap_Resolve_Unknown(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionNone, km.Resolve(keys.ContextChatList, "F9"))
}
