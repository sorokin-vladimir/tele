package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// errRefused stands in for whatever Telegram refused with. The taxonomy is
// mapped in internal/tg, so the core only has to propagate.
var errRefused = errors.New("upload refused")

func TestUploadPart_AnImageBecomesAnUploadedPhoto(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	part := domain.OutboxMediaPart{Path: writeFile(t, "a.jpg", 4), Name: "a.jpg"}

	got, err := o.uploadPart(context.Background(), part, nil)

	require.NoError(t, err)
	assert.IsType(t, &tg.InputMediaUploadedPhoto{}, got)
	assert.Equal(t, []string{part.Path}, c.uploads())
}

func TestUploadPart_SendAsFileOverridesTheDetectedKind(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	part := domain.OutboxMediaPart{
		Path: writeFile(t, "a.jpg", 4), Name: "a.jpg", SendAs: domain.MediaFile,
	}

	got, err := o.uploadPart(context.Background(), part, nil)

	require.NoError(t, err)
	doc, ok := got.(*tg.InputMediaUploadedDocument)
	require.True(t, ok, "the user asked for a file, not a photo")
	assert.Equal(t, "image/jpeg", doc.MimeType)
	assert.Equal(t, "a.jpg", documentFileName(t, doc))
	assert.True(t, doc.ForceFile,
		"without ForceFile Telegram reinterprets an image as a photo, undoing the choice")
}

// Video is a document too, but a streamable one: ForceFile would make Telegram
// render it as an attachment instead of playing it inline.
func TestUploadPart_AVideoIsSentAsStreamableVideo(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	part := domain.OutboxMediaPart{Path: writeFile(t, "clip.mp4", 4), Name: "clip.mp4"}

	got, err := o.uploadPart(context.Background(), part, nil)

	require.NoError(t, err)
	doc, ok := got.(*tg.InputMediaUploadedDocument)
	require.True(t, ok, "got %T", got)
	assert.False(t, doc.ForceFile)
	assert.Equal(t, "video/mp4", doc.MimeType)
	var hasVideoAttr bool
	for _, a := range doc.Attributes {
		if _, ok := a.(*tg.DocumentAttributeVideo); ok {
			hasVideoAttr = true
		}
	}
	assert.True(t, hasVideoAttr, "a video needs its video attribute to play inline")
}

func TestUploadPart_AnUnknownTypeBecomesADocument(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	part := domain.OutboxMediaPart{Path: writeFile(t, "notes.txt", 4), Name: "notes.txt"}

	got, err := o.uploadPart(context.Background(), part, nil)

	require.NoError(t, err)
	assert.IsType(t, &tg.InputMediaUploadedDocument{}, got)
}

func TestUploadPart_ReportsProgress(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	part := domain.OutboxMediaPart{Path: writeFile(t, "a.jpg", 4), Name: "a.jpg"}

	var last int64
	_, err := o.uploadPart(context.Background(), part, func(sent, _ int64) { last = sent })

	require.NoError(t, err)
	assert.Equal(t, int64(100), last)
}

func TestUploadPart_AFailedUploadIsReturned(t *testing.T) {
	c := &stubClient{uploadErr: errRefused}
	o, _ := newCmdOwner(t, c)
	part := domain.OutboxMediaPart{Path: writeFile(t, "a.jpg", 4), Name: "a.jpg"}

	_, err := o.uploadPart(context.Background(), part, nil)

	require.ErrorIs(t, err, errRefused)
}

// The reported case (#224, #230): a JPEG saved by a browser with no extension
// in its name. The bytes are a photo, detection says so, and the send used to
// go up under the bare name and come back PHOTO_EXT_INVALID.
func TestUploadPart_AnExtensionlessImageGoesUpUnderARepairedName(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	part := domain.OutboxMediaPart{Path: writeJPEG(t, "0f3a9c"), Name: "0f3a9c"}

	got, err := o.uploadPart(context.Background(), part, nil)

	require.NoError(t, err)
	assert.IsType(t, &tg.InputMediaUploadedPhoto{}, got, "the bytes are a photo and it is sent as one")
	assert.Equal(t, []string{"0f3a9c.jpg"}, c.uploadNames())
	assert.Equal(t, "0f3a9c", part.Name, "the name the person picked is not ours to rewrite")
}

// The upload name is protocol detail. What the recipient sees is part.Name, and
// the repair must not reach it.
func TestUploadPart_TheRecipientKeepsTheNameWithoutTheExtension(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	part := domain.OutboxMediaPart{
		Path: writeJPEG(t, "0f3a9c"), Name: "0f3a9c", SendAs: domain.MediaFile,
	}

	got, err := o.uploadPart(context.Background(), part, nil)

	require.NoError(t, err)
	doc, ok := got.(*tg.InputMediaUploadedDocument)
	require.True(t, ok, "got %T", got)
	assert.Equal(t, "0f3a9c", documentFileName(t, doc))
	assert.Equal(t, []string{"0f3a9c.jpg"}, c.uploadNames())
}

// A name that says something is believed. Sniffing every file to catch a
// mislabelled one would change the whole send path for a case nobody has hit,
// and Telegram re-encodes photos past it anyway.
func TestUploadPart_AWrongExtensionIsLeftAlone(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	part := domain.OutboxMediaPart{Path: writeJPEG(t, "a.png"), Name: "a.png"}

	_, err := o.uploadPart(context.Background(), part, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"a.png"}, c.uploadNames())
}

// Nothing detected means nothing to derive an extension from. The name goes as
// it is, the file goes as a document, and a refusal is the net (#224).
func TestUploadPart_AnUndetectableTypeKeepsItsName(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)
	part := domain.OutboxMediaPart{Path: writeFile(t, "blob", 4), Name: "blob"}

	got, err := o.uploadPart(context.Background(), part, nil)

	require.NoError(t, err)
	assert.IsType(t, &tg.InputMediaUploadedDocument{}, got)
	assert.Equal(t, []string{"blob"}, c.uploadNames())
}

// writeJPEG writes a file whose first bytes are a JPEG's, so detection has
// something real to read when the name tells it nothing.
func writeJPEG(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, 0o600))
	return path
}

// documentFileName digs the file name out of a document's attributes, which is
// where Telegram keeps it.
func documentFileName(t *testing.T, doc *tg.InputMediaUploadedDocument) string {
	t.Helper()
	for _, a := range doc.Attributes {
		if fn, ok := a.(*tg.DocumentAttributeFilename); ok {
			return fn.FileName
		}
	}
	return ""
}
