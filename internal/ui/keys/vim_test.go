package keys_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

func TestVimState_BasicMotions(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ActionDown, vs.Process("j"))
	assert.Equal(t, keys.ActionUp, vs.Process("k"))
	assert.Equal(t, keys.ActionLeft, vs.Process("h"))
	assert.Equal(t, keys.ActionRight, vs.Process("l"))
}

func TestVimState_GG(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ActionNone, vs.Process("g"))
	assert.Equal(t, keys.ActionGoTop, vs.Process("g"))
}

func TestVimState_GUppercase(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ActionGoBottom, vs.Process("G"))
}

func TestVimState_InsertMode(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ActionInsert, vs.Process("i"))
	assert.Equal(t, keys.ModeInsert, vs.Mode)
	assert.Equal(t, keys.ActionPassthrough, vs.Process("j"))
	assert.Equal(t, keys.ActionNormal, vs.Process("esc"))
	assert.Equal(t, keys.ModeNormal, vs.Mode)
}

func TestVimState_Enter(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ActionConfirm, vs.Process("enter"))
}

func TestVimState_Slash(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ActionSearch, vs.Process("/"))
}

func TestVimState_UnknownKey(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ActionNone, vs.Process("x"))
}

func TestVimState_GG_InvalidSecond(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ActionNone, vs.Process("g"))
	assert.Equal(t, keys.ActionNone, vs.Process("x"))
	assert.Equal(t, keys.ActionDown, vs.Process("j"))
}

func TestVimState_Space_OpenContextMenu(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ActionOpenContextMenu, vs.Process("space"))
}

func TestVimState_Space_PassthroughInInsertMode(t *testing.T) {
	vs := keys.NewVimState()
	vs.Process("i") // enter insert mode
	assert.Equal(t, keys.ActionPassthrough, vs.Process("space"))
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

func TestVimState_R_NormalMode_Reply(t *testing.T) {
	vs := keys.NewVimState()
	assert.Equal(t, keys.ActionReply, vs.Process("r"))
}

func TestVimState_R_InsertMode_Passthrough(t *testing.T) {
	vs := keys.NewVimState()
	vs.Process("i") // enter insert mode
	assert.Equal(t, keys.ActionPassthrough, vs.Process("r"))
}
