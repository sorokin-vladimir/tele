//go:build darwin

package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOsascriptInfoCmd_Args(t *testing.T) {
	cmd := osascriptInfoCmd()
	assert.Equal(t, "osascript", cmd.Path[strings.LastIndex(cmd.Path, "/")+1:])
	assert.Equal(t, []string{"osascript", "-e", "clipboard info"}, cmd.Args)
}

func TestOsascriptExtractCmd_WritesPngToPath(t *testing.T) {
	cmd := osascriptExtractCmd("/tmp/clip.png")
	joined := strings.Join(cmd.Args, "\n")
	// Coerces the clipboard to PNG and writes it to the given POSIX path.
	assert.Contains(t, joined, "«class PNGf»")
	assert.Contains(t, joined, `POSIX file "/tmp/clip.png"`)
	assert.Contains(t, joined, "set eof fp to 0") // truncate before writing
}
