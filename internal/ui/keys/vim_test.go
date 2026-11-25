package keys_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
)

func TestVimState_StartsInNormalMode(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ModeNormal, vs.Mode)
}

func TestDefaultKeyMap_ContextContextMenu(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionDown, km.Resolve(keys.ContextContextMenu, "j"))
	assert.Equal(t, keys.ActionDown, km.Resolve(keys.ContextContextMenu, "down"))
	assert.Equal(t, keys.ActionUp, km.Resolve(keys.ContextContextMenu, "k"))
	assert.Equal(t, keys.ActionUp, km.Resolve(keys.ContextContextMenu, "up"))
	assert.Equal(t, keys.ActionConfirm, km.Resolve(keys.ContextContextMenu, "enter"))
	assert.Equal(t, keys.ActionCancel, km.Resolve(keys.ContextContextMenu, "esc"))
	assert.Equal(t, keys.ActionCancel, km.Resolve(keys.ContextContextMenu, "space"))
	assert.Equal(t, keys.ActionReply, km.Resolve(keys.ContextContextMenu, "r"))
	assert.Equal(t, keys.ActionReact, km.Resolve(keys.ContextContextMenu, "t"))
	assert.Equal(t, keys.ActionEdit, km.Resolve(keys.ContextContextMenu, "e"))
	assert.Equal(t, keys.ActionDelete, km.Resolve(keys.ContextContextMenu, "d"))
}

func TestDefaultKeyMap_ContextDeleteSubMenu(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionDown, km.Resolve(keys.ContextDeleteSubMenu, "j"))
	assert.Equal(t, keys.ActionUp, km.Resolve(keys.ContextDeleteSubMenu, "k"))
	assert.Equal(t, keys.ActionConfirm, km.Resolve(keys.ContextDeleteSubMenu, "enter"))
	assert.Equal(t, keys.ActionCancel, km.Resolve(keys.ContextDeleteSubMenu, "esc"))
	assert.Equal(t, keys.ActionDeleteRevoke, km.Resolve(keys.ContextDeleteSubMenu, "a"))
	assert.Equal(t, keys.ActionDeleteMe, km.Resolve(keys.ContextDeleteSubMenu, "m"))
}

func TestDefaultKeyMap_ContextSearch(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionCancel, km.Resolve(keys.ContextSearch, "esc"))
	assert.Equal(t, keys.ActionConfirm, km.Resolve(keys.ContextSearch, "enter"))
	assert.Equal(t, keys.ActionDown, km.Resolve(keys.ContextSearch, "down"))
	assert.Equal(t, keys.ActionDown, km.Resolve(keys.ContextSearch, "ctrl+j"))
	assert.Equal(t, keys.ActionUp, km.Resolve(keys.ContextSearch, "up"))
	assert.Equal(t, keys.ActionUp, km.Resolve(keys.ContextSearch, "ctrl+k"))
}

// `/` finds a chat by name everywhere but the chat pane, where it searches the
// open chat's messages; `g /` searches every chat's messages from anywhere, and
// Tab in a search window switches where it looks (#349).
func TestDefaultKeyMap_Searches(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionChatSearch, km.Resolve(keys.ContextGlobal, "/"))
	assert.Equal(t, keys.ActionChatSearch, km.Resolve(keys.ContextChatList, "/"), "the chat list takes / from global")
	assert.Equal(t, keys.ActionChatSearch, km.Resolve(keys.ContextFolders, "/"), "the folders take / from global")
	assert.Equal(t, keys.ActionMessageSearch, km.Resolve(keys.ContextChat, "/"))
	assert.Equal(t, keys.ActionMessageSearchAll, km.Resolve(keys.ContextGlobal, "g /"))
	assert.Equal(t, keys.ActionToggleScope, km.Resolve(keys.ContextSearch, "tab"))
}

func TestDefaultKeyMap_ContextComposer(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionConfirm, km.Resolve(keys.ContextComposer, "enter"))
	assert.Equal(t, keys.ActionNormal, km.Resolve(keys.ContextComposer, "esc"))
}

func TestKeyMap_DownloadFile_BoundInChatAndMenu(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionDownloadFile, km.Resolve(keys.ContextChat, "s"))
	assert.Equal(t, keys.ActionDownloadFile, km.Resolve(keys.ContextContextMenu, "s"))
}

func TestKeyMap_DownloadFile_RussianLayout(t *testing.T) {
	km := keys.DefaultKeyMap()
	// 'ы' sits on the physical 's' key under the Russian layout.
	assert.Equal(t, keys.ActionDownloadFile, km.Resolve(keys.ContextChat, "ы"))
	assert.Equal(t, keys.ActionDownloadFile, km.Resolve(keys.ContextContextMenu, "ы"))
}
