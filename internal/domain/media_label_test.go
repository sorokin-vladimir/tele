package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// A piece of media is named by its kind and what it says about itself, with
// nothing fetched: a command prints it and a search window shows it in place
// of a text the message does not have (#349).
func TestMediaRef_Label(t *testing.T) {
	cases := map[string]domain.MediaRef{
		"photo":           {Kind: domain.MediaPhoto},
		"voice 1:12":      {Kind: domain.MediaVoice, Duration: 72},
		"video 1:02:05":   {Kind: domain.MediaVideo, Duration: 3725},
		"file report.pdf": {Kind: domain.MediaFile, FileName: "report.pdf"},
		"audio Nina – Song 3:05": {
			Kind: domain.MediaAudio, Duration: 185, Performer: "Nina", Title: "Song",
		},
	}
	for want, r := range cases {
		assert.Equal(t, want, r.Label())
	}
}
