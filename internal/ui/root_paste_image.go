package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// clipboardImagePastedMsg reports the outcome of a composer Ctrl+V that tried to
// read an image: path is a staged temp file, or err describes an extraction
// failure. Both empty means the branch produced a plain text paste instead.
type clipboardImagePastedMsg struct {
	path string
	err  error
}

// readClipboardForComposerCmd runs off the update loop. It prefers a clipboard
// image (staged as a photo); on no image it falls back to today's text paste;
// on a reader failure it reports the error so the handler can toast + fall back.
// A single tea.Cmd yields one message, so the error path defers the text
// fallback to the handler.
func readClipboardForComposerCmd(tmpDir string) tea.Cmd {
	return func() tea.Msg {
		data, ext, err := clipImageReader.ReadImage()
		if err != nil {
			return clipboardImagePastedMsg{err: err}
		}
		if len(data) == 0 {
			// No image: preserve the existing text-paste behavior.
			str, terr := clipboardRead()
			if terr != nil || str == "" {
				return nil
			}
			return tea.PasteMsg{Content: str}
		}
		path, werr := writeTempMediaFile(data, tmpDir, ext)
		if werr != nil {
			return clipboardImagePastedMsg{err: werr}
		}
		return clipboardImagePastedMsg{path: path}
	}
}

// handleClipboardImagePasted stages a pasted clipboard image as a photo, or on
// failure shows a toast and falls back to a text paste.
func (m RootModel) handleClipboardImagePasted(msg clipboardImagePastedMsg) (RootModel, tea.Cmd) {
	if msg.err != nil {
		toast := func() tea.Msg {
			return StatusErrMsg{Text: "clipboard image paste failed", Sev: components.SeverityWarning}
		}
		return m, tea.Batch(toast, readClipboardCmd())
	}
	if msg.path == "" {
		return m, nil
	}
	return m.stageAttachmentFromPath(msg.path)
}
