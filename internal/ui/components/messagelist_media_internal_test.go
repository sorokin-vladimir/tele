package components

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

func TestRenderUploadBar(t *testing.T) {
	bar := renderUploadBar(0.6, 20)
	if !strings.Contains(bar, "60%") {
		t.Fatalf("bar missing percent: %q", bar)
	}
}

func TestUploadStatusLineUploading(t *testing.T) {
	s := uploadStatusLine(&domain.LocalMedia{UploadProgress: 0.4, Parts: 1}, 20)
	if !strings.Contains(s, "40%") {
		t.Fatalf("uploading status missing percent: %q", s)
	}
}

// A group counts its files; a lone one has nothing to count. The counter rides
// on the label, which is what the bubble is measured by.
func TestLocalMediaLabelCountsTheFilesOfAGroup(t *testing.T) {
	s := localMediaLabel(&domain.LocalMedia{Kind: domain.MediaPhoto, Part: 2, Parts: 3})
	if !strings.Contains(s, "2/3") {
		t.Fatalf("group label missing the file counter: %q", s)
	}
	lone := localMediaLabel(&domain.LocalMedia{Kind: domain.MediaPhoto, FileName: "a.jpg", Part: 1, Parts: 1})
	if strings.Contains(lone, "1/1") {
		t.Fatalf("a single file should not be counted: %q", lone)
	}
}

// The label sizes the bubble, so it must not change width as the upload runs:
// every step of a ten-part send has to render the same number of cells.
func TestLocalMediaLabelKeepsAConstantWidthThroughTheSend(t *testing.T) {
	want := lipgloss.Width(localMediaLabel(&domain.LocalMedia{Kind: domain.MediaPhoto, Parts: 10}))
	for part := 1; part <= 10; part++ {
		lm := &domain.LocalMedia{Kind: domain.MediaPhoto, Part: part, Parts: 10}
		if got := lipgloss.Width(localMediaLabel(lm)); got != want {
			t.Fatalf("label at %d/10 is %d cells, want %d: %q", part, got, want, localMediaLabel(lm))
		}
	}
}

// Keep upload progress readable without relying on label truncation to fit it.
func TestUploadStatusLineNeverExceedsTheWidthItIsGiven(t *testing.T) {
	for _, width := range []int{8, 12, 20, 40} {
		lm := &domain.LocalMedia{Kind: domain.MediaPhoto, UploadProgress: 0.42, Part: 3, Parts: 5}
		if got := lipgloss.Width(uploadStatusLine(lm, width)); got > width {
			t.Fatalf("status line is %d cells wide, was given %d", got, width)
		}
	}
}

func TestLocalMediaLabel_Photo(t *testing.T) {
	got := localMediaLabel(&domain.LocalMedia{Kind: domain.MediaPhoto, FileName: "pic.jpg"})
	if !strings.HasPrefix(got, "🖼") || !strings.Contains(got, "pic.jpg") {
		t.Fatalf("photo label want 🖼 + name, got %q", got)
	}
}

func TestLocalMediaLabel_File(t *testing.T) {
	got := localMediaLabel(&domain.LocalMedia{Kind: domain.MediaFile, FileName: "report.pdf"})
	if !strings.HasPrefix(got, "📎") || !strings.Contains(got, "report.pdf") {
		t.Fatalf("file label want 📎 + name, got %q", got)
	}
}

func TestVideoOverlayLabel_GIF(t *testing.T) {
	if got := videoOverlayLabel(&domain.MediaRef{Kind: domain.MediaGIF}); got != "GIF" {
		t.Fatalf("GIF overlay want \"GIF\", got %q", got)
	}
}

func TestVideoOverlayLabel_PhotoEmpty(t *testing.T) {
	if got := videoOverlayLabel(&domain.MediaRef{Kind: domain.MediaPhoto}); got != "" {
		t.Fatalf("photo must have no overlay label, got %q", got)
	}
}

func TestOverlayLabelFor_GIFLoadingSpinner(t *testing.T) {
	ml := NewMessageList(20, 40)
	gif := domain.Message{
		Media:    &domain.MediaRef{Kind: domain.MediaGIF},
		Document: &domain.DocumentRef{ID: 5, ThumbSize: "m"},
	}
	if got := ml.overlayLabelFor(gif); got != "GIF" {
		t.Fatalf("idle GIF want \"GIF\", got %q", got)
	}
	ml.SetGifLoading(5, "⠋")
	if got := ml.overlayLabelFor(gif); got != "⠋ GIF" {
		t.Fatalf("loading GIF want \"⠋ GIF\", got %q", got)
	}
	ml.SetGifLoading(99, "⠋") // a different gif is loading
	if got := ml.overlayLabelFor(gif); got != "GIF" {
		t.Fatalf("non-loading GIF want \"GIF\", got %q", got)
	}
}

func TestSelectedMessageGIF(t *testing.T) {
	ml := NewMessageList(20, 40)
	ml.SetMessages([]domain.Message{{
		ID:       1,
		Media:    &domain.MediaRef{Kind: domain.MediaGIF},
		Document: &domain.DocumentRef{ID: 55, ThumbSize: "m"},
	}})
	ref, ok := ml.SelectedMessageGIF()
	if !ok || ref.ID != 55 {
		t.Fatalf("got (id=%d, ok=%v), want (55, true)", ref.ID, ok)
	}
}

func TestSelectedMessageGIF_NotAGif(t *testing.T) {
	ml := NewMessageList(20, 40)
	ml.SetMessages([]domain.Message{{
		ID:    1,
		Media: &domain.MediaRef{Kind: domain.MediaPhoto},
		Photo: &domain.PhotoRef{ID: 9},
	}})
	if _, ok := ml.SelectedMessageGIF(); ok {
		t.Fatal("photo selection must not report a GIF")
	}
}

func TestPreviewImageID_GIFWithThumb(t *testing.T) {
	ml := NewMessageList(20, 40)
	msg := domain.Message{
		Media:    &domain.MediaRef{Kind: domain.MediaGIF},
		Document: &domain.DocumentRef{ID: 777, ThumbSize: "m"},
	}
	id, ok := ml.PreviewImageID(msg)
	if !ok || id != 777 {
		t.Fatalf("GIF with thumb: got (id=%d, ok=%v), want (777, true)", id, ok)
	}
}

func TestPreviewImageID_GIFWithoutThumb(t *testing.T) {
	ml := NewMessageList(20, 40)
	msg := domain.Message{
		Media:    &domain.MediaRef{Kind: domain.MediaGIF},
		Document: &domain.DocumentRef{ID: 777}, // no ThumbSize
	}
	if _, ok := ml.PreviewImageID(msg); ok {
		t.Fatal("GIF without a thumb must have no inline preview")
	}
}

func TestLocalMediaLabel_Video(t *testing.T) {
	got := localMediaLabel(&domain.LocalMedia{Kind: domain.MediaVideo, FileName: "clip.mp4"})
	if got != "🎥 clip.mp4" {
		t.Fatalf("video label want '🎥 clip.mp4', got %q", got)
	}
}

func TestLocalMediaLabel_VideoNoName(t *testing.T) {
	got := localMediaLabel(&domain.LocalMedia{Kind: domain.MediaVideo})
	if got != "🎥 video" {
		t.Fatalf("nameless video label want '🎥 video', got %q", got)
	}
}

func TestPlaceholderFor_FileWithNameAndSize(t *testing.T) {
	got := placeholderFor(&domain.MediaRef{Kind: domain.MediaFile, FileName: "report.pdf", Size: 1300000})
	if !strings.Contains(got, "report.pdf") || !strings.Contains(got, "MB") {
		t.Fatalf("file placeholder want name + size, got %q", got)
	}
}

func TestPlaceholderFor_FileNoName(t *testing.T) {
	got := placeholderFor(&domain.MediaRef{Kind: domain.MediaFile})
	if got != "📎 file" {
		t.Fatalf("nameless file placeholder want '📎 file', got %q", got)
	}
}
