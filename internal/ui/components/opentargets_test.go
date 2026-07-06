package components_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageOpenTargets_PhotoOnly(t *testing.T) {
	msg := store.Message{Photo: &store.PhotoRef{ID: 1}}
	got := components.MessageOpenTargets(msg)
	require.Len(t, got, 1)
	assert.Equal(t, components.OpenTargetPhoto, got[0].Kind)
	assert.Equal(t, "Photo", got[0].Label)
}

func TestMessageOpenTargets_Video(t *testing.T) {
	msg := store.Message{Media: &store.MediaRef{Kind: store.MediaVideo}, Document: &store.DocumentRef{ID: 2}}
	got := components.MessageOpenTargets(msg)
	require.Len(t, got, 1)
	assert.Equal(t, components.OpenTargetVideo, got[0].Kind)
	assert.Equal(t, "Video", got[0].Label)
}

func TestMessageOpenTargets_VideoNote_IsVideoTarget(t *testing.T) {
	msg := store.Message{Media: &store.MediaRef{Kind: store.MediaVideoNote}, Document: &store.DocumentRef{ID: 3}}
	got := components.MessageOpenTargets(msg)
	require.Len(t, got, 1)
	assert.Equal(t, components.OpenTargetVideo, got[0].Kind)
}

func TestMessageOpenTargets_Voice_NoTargets(t *testing.T) {
	msg := store.Message{Media: &store.MediaRef{Kind: store.MediaVoice}, Document: &store.DocumentRef{ID: 4}}
	assert.Empty(t, components.MessageOpenTargets(msg))
}

func TestMessageOpenTargets_PlainURL(t *testing.T) {
	msg := store.Message{Text: "see https://example.com now",
		Entities: []store.MessageEntity{{Type: "url", Offset: 4, Length: 19}}}
	got := components.MessageOpenTargets(msg)
	require.Len(t, got, 1)
	assert.Equal(t, components.OpenTargetLink, got[0].Kind)
	assert.Equal(t, "https://example.com", got[0].Label)
	assert.Equal(t, "https://example.com", got[0].URL)
}

func TestMessageOpenTargets_SchemelessURL_NormalizedTarget(t *testing.T) {
	msg := store.Message{Text: "visit example.com today",
		Entities: []store.MessageEntity{{Type: "url", Offset: 6, Length: 11}}}
	got := components.MessageOpenTargets(msg)
	require.Len(t, got, 1)
	assert.Equal(t, "example.com", got[0].Label, "label is the text as typed")
	assert.Equal(t, "https://example.com", got[0].URL, "target gets https scheme")
}

func TestMessageOpenTargets_Email_Mailto(t *testing.T) {
	msg := store.Message{Text: "mail a@b.com",
		Entities: []store.MessageEntity{{Type: "email", Offset: 5, Length: 7}}}
	got := components.MessageOpenTargets(msg)
	require.Len(t, got, 1)
	assert.Equal(t, "a@b.com", got[0].Label)
	assert.Equal(t, "mailto:a@b.com", got[0].URL)
}

func TestMessageOpenTargets_TextURL_LabelShowsTextArrowURL(t *testing.T) {
	msg := store.Message{Text: "click here",
		Entities: []store.MessageEntity{{Type: "text_url", Offset: 0, Length: 5, URL: "https://ex.com/y"}}}
	got := components.MessageOpenTargets(msg)
	require.Len(t, got, 1)
	assert.Equal(t, "click → https://ex.com/y", got[0].Label)
	assert.Equal(t, "https://ex.com/y", got[0].URL)
}

func TestMessageOpenTargets_Phone_NotATarget(t *testing.T) {
	msg := store.Message{Text: "call 12345",
		Entities: []store.MessageEntity{{Type: "phone", Offset: 5, Length: 5}}}
	assert.Empty(t, components.MessageOpenTargets(msg))
}

func TestMessageOpenTargets_PhotoThenLinks_MediaFirst(t *testing.T) {
	msg := store.Message{
		Text:     "pic https://a.com and https://b.com",
		Photo:    &store.PhotoRef{ID: 9},
		Entities: []store.MessageEntity{{Type: "url", Offset: 4, Length: 13}, {Type: "url", Offset: 22, Length: 13}},
	}
	got := components.MessageOpenTargets(msg)
	require.Len(t, got, 3)
	assert.Equal(t, components.OpenTargetPhoto, got[0].Kind)
	assert.Equal(t, "https://a.com", got[1].URL)
	assert.Equal(t, "https://b.com", got[2].URL)
}

func TestMessageOpenTargets_PlainText_NoTargets(t *testing.T) {
	assert.Empty(t, components.MessageOpenTargets(store.Message{Text: "just text"}))
}
