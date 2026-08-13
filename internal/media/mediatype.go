// Package media provides MIME detection and MIME-based classification helpers
// shared by the outbound (upload) and, later, the download/open paths. Beyond
// domain.MediaKind it depends on the standard library and on the mimetype table
// alone, so both directions agree on how MIME strings are normalized, mapped to
// a kind and named with an extension.
package media

import (
	"strings"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// NormalizeMIME lowercases a MIME type and strips any parameters (everything from
// the first ";") and surrounding whitespace, so callers compare bare types.
func NormalizeMIME(mime string) string {
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	return strings.ToLower(strings.TrimSpace(mime))
}

// DefaultMediaType maps a MIME type to the default domain.MediaKind for a picked
// file. It is a default only: intent (e.g. send a .jpg "as photo" vs "as file") is
// resolved later on the confirm screen (#106). audio/ogg voice notes map to Voice;
// other audio maps to Audio.
func DefaultMediaType(mime string) domain.MediaKind {
	m := NormalizeMIME(mime)
	switch {
	case strings.HasPrefix(m, "image/"):
		return domain.MediaPhoto
	case strings.HasPrefix(m, "video/"):
		return domain.MediaVideo
	case m == "audio/ogg":
		return domain.MediaVoice
	case strings.HasPrefix(m, "audio/"):
		return domain.MediaAudio
	default:
		return domain.MediaFile
	}
}
