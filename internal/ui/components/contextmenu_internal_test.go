package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenItemLabel_SingleTarget_Specialized(t *testing.T) {
	assert.Equal(t, "Open photo", openItemLabel([]OpenTarget{{Kind: OpenTargetPhoto}}))
	assert.Equal(t, "Open video", openItemLabel([]OpenTarget{{Kind: OpenTargetVideo}}))
	assert.Equal(t, "Open link", openItemLabel([]OpenTarget{{Kind: OpenTargetLink}}))
}

func TestOpenItemLabel_MultipleTargets_Generic(t *testing.T) {
	label := openItemLabel([]OpenTarget{{Kind: OpenTargetPhoto}, {Kind: OpenTargetLink}})
	assert.Equal(t, "Open", label)
}
