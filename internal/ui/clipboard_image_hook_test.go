package ui_test

import (
	"errors"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/stretchr/testify/assert"
)

func TestSetClipboardImageReaderForTest_SwapsAndRestores(t *testing.T) {
	restore := ui.SetClipboardImageReaderForTest(func() ([]byte, string, error) {
		return []byte("PNGBYTES"), ".png", nil
	})
	data, ext, err := ui.ReadClipboardImageForTest()
	assert.NoError(t, err)
	assert.Equal(t, []byte("PNGBYTES"), data)
	assert.Equal(t, ".png", ext)

	restore()
	// After restore the default reader is back; a fresh swap to an error reader
	// proves the var is writable again (does not touch the live clipboard).
	ui.SetClipboardImageReaderForTest(func() ([]byte, string, error) {
		return nil, "", errors.New("boom")
	})
	_, _, err = ui.ReadClipboardImageForTest()
	assert.Error(t, err)
}
