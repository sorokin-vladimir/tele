package ui

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/media"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// pendingAttachment is the staged file awaiting send. kind is the MIME-detected
// default; sendAs is the user's choice (Photo/File toggle). The File branch is #129.
type pendingAttachment struct {
	path   string
	mime   string
	kind   store.MediaKind
	sendAs store.MediaKind
	name   string
	size   int64
}

func (m RootModel) openFilePicker() (RootModel, tea.Cmd) {
	m.filePicker = screens.NewFilePickerModel(m.lastPickerDir, m.width, m.height, m.keyMap)
	m.statusBar.SetPickerOpen(true)
	return m, nil
}

func (m RootModel) handleFileSelected(msg screens.FileSelectedMsg) (RootModel, tea.Cmd) {
	if m.filePicker != nil {
		m.lastPickerDir = m.filePicker.Dir()
	}
	m.filePicker = nil
	m.statusBar.SetPickerOpen(false)
	return m.stageAttachmentFromPath(msg.Path)
}

// stageAttachmentFromPath stages a local file as a pending attachment: it MIME-
// detects the kind, shows the composer chip, enters insert mode so the caption
// field is active, and focuses the composer. Shared by the file picker and the
// clipboard-image paste (#163). Photo/File is toggleable for image/video.
func (m RootModel) stageAttachmentFromPath(path string) (RootModel, tea.Cmd) {
	mime, err := media.DetectMIME(path)
	if err != nil {
		return m, func() tea.Msg {
			return StatusErrMsg{Text: "cannot read file", Sev: components.SeverityWarning}
		}
	}
	kind := media.DefaultMediaType(mime)
	name, size := fileNameSize(path)
	m.pendingAttachment = &pendingAttachment{
		path:   path,
		mime:   mime,
		kind:   kind,
		sendAs: kind,
		name:   name,
		size:   size,
	}
	toggleable := kind == store.MediaPhoto || kind == store.MediaVideo
	m.chat.SetAttachment(name, size, m.pendingAttachment.kind, m.pendingAttachment.sendAs, toggleable)
	m.statusBar.SetAttachStaged(true)
	// Enter real insert mode so the caption field is active (the composer focus
	// alone does not flip the root's vim mode, which key routing depends on).
	m.vimState.Mode = keys.ModeInsert
	m.statusBar.SetMode(keys.ModeInsert)
	// Attaching drops the limit from a message's 4096 to a caption's 1024, which
	// can leave an existing draft over the limit (#126).
	focusCmd := m.chat.FocusComposer()
	if m.chat.ComposerOverLimit() {
		var toastCmd tea.Cmd
		m, toastCmd = m.handleComposerLimit(components.ComposerLimitMsg{
			Kind: components.ComposerLimitOver, Limit: maxCaptionChars, Caption: true,
		})
		return m, tea.Batch(focusCmd, toastCmd)
	}
	return m, focusCmd
}

// PendingAttachmentSendAs reports the staged "send as" kind (test accessor).
func (m RootModel) PendingAttachmentSendAs() (store.MediaKind, bool) {
	if m.pendingAttachment == nil {
		return 0, false
	}
	return m.pendingAttachment.sendAs, true
}

// toggleSendAs flips the staged attachment between its native kind and File.
// Only image/video are toggleable; the File branch hands off to #129.
func (m RootModel) toggleSendAs() (RootModel, tea.Cmd) {
	if m.pendingAttachment == nil {
		return m, nil
	}
	if m.pendingAttachment.kind != store.MediaPhoto && m.pendingAttachment.kind != store.MediaVideo {
		return m, nil
	}
	if m.pendingAttachment.sendAs == store.MediaFile {
		m.pendingAttachment.sendAs = m.pendingAttachment.kind
	} else {
		m.pendingAttachment.sendAs = store.MediaFile
	}
	m.chat.SetAttachment(m.pendingAttachment.name, m.pendingAttachment.size, m.pendingAttachment.kind, m.pendingAttachment.sendAs, true)
	return m, nil
}

func (m *RootModel) clearPendingAttachment() {
	m.pendingAttachment = nil
	m.chat.ClearAttachment()
	m.statusBar.SetAttachStaged(false)
}

func fileNameSize(path string) (string, int64) {
	name := filepath.Base(path)
	var size int64
	if fi, err := os.Stat(path); err == nil {
		size = fi.Size()
	}
	return name, size
}
