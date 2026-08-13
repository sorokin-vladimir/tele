package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtensionFor(t *testing.T) {
	cases := []struct {
		name  string
		mime  string
		ext   string
		known bool
	}{
		{"a photo names the extension Telegram expects", "image/jpeg", ".jpg", true},
		{"png", "image/png", ".png", true},
		{"video", "video/mp4", ".mp4", true},
		// The sniffer's answer for bytes it could not place. The type is real
		// and carries no extension, which is different from never having heard
		// of it: there is nothing to append, and nothing was misread.
		{"unplaced bytes", "application/octet-stream", "", true},
		{"a type we do not know", "application/weird", "", false},
		{"nothing detected at all", "", "", false},
		// The parameters and the casing come off before the lookup, so a type
		// straight from a header resolves the same as a bare one.
		{"a type with parameters", "Image/JPEG; charset=binary", ".jpg", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ext, known := ExtensionFor(c.mime)
			assert.Equal(t, c.ext, ext)
			assert.Equal(t, c.known, known)
		})
	}
}
