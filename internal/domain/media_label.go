package domain

import (
	"fmt"
	"strings"
)

// Label names a piece of media and what it says about itself, without
// fetching it: "photo", "voice 1:12", "file report.pdf".
func (r MediaRef) Label() string {
	withTime := func(label string) string {
		return strings.TrimSpace(label + " " + clock(r.Duration))
	}
	//exhaustive:enforce
	switch r.Kind {
	case MediaPhoto:
		return "photo"
	case MediaVideo:
		return withTime("video")
	case MediaVideoNote:
		return withTime("round video")
	case MediaVoice:
		return withTime("voice")
	case MediaAudio:
		name := r.FileName
		if r.Title != "" {
			name = strings.TrimPrefix(r.Performer+" – "+r.Title, " – ")
		}
		return withTime(strings.TrimSpace("audio " + name))
	case MediaSticker:
		return strings.TrimSpace("sticker " + r.Emoji)
	case MediaGIF:
		return "GIF"
	case MediaFile:
		return strings.TrimSpace("file " + r.FileName)
	case MediaLocation:
		return "location"
	case MediaOther:
		fallthrough
	default:
		return "media"
	}
}

// clock is a duration in seconds as a player shows it, m:ss or h:mm:ss; empty
// for none.
func clock(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	h, m, s := seconds/3600, seconds/60%60, seconds%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
