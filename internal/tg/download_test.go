package tg

import (
	"errors"
	"testing"

	"github.com/gotd/td/tgerr"
	"github.com/stretchr/testify/assert"
)

func TestIsFileReferenceExpired(t *testing.T) {
	expired := &tgerr.Error{Code: 400, Type: "FILE_REFERENCE_EXPIRED"}
	assert.True(t, IsFileReferenceExpired(expired))
	assert.False(t, IsFileReferenceExpired(errors.New("boom")))
	assert.False(t, IsFileReferenceExpired(nil))
}
