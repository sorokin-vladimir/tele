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
