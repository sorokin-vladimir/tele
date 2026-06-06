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

func TestDefaultKeyMap_ContextComposer(t *testing.T) {
	km := keys.DefaultKeyMap()
	assert.Equal(t, keys.ActionConfirm, km.Resolve(keys.ContextComposer, "enter"))
	assert.Equal(t, keys.ActionNormal, km.Resolve(keys.ContextComposer, "esc"))
}
