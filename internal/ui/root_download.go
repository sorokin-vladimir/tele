package ui

import (
	"context"
	"image"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// decodeImageFile decodes an image the owner cached for us. A file that has
// vanished (evicted between the fetch and the open) and a file that will not
// decode are both errors here; the caller decides what to do about it.
func decodeImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// fetchInlineImageCmd fetches, decodes and hands over one inline image.
// transform is applied to the decoded image (round notes are cropped to a
// circle) and may be nil. An undecodable file is dropped from the owner's cache
// so the next repaint tries again instead of getting the same broken bytes back.
func fetchInlineImageCmd(ctx context.Context, o Owner, chatID int64, msgID int, slot domain.MediaSlot, imageID int64, action string, transform func(image.Image) image.Image) tea.Cmd {
	return func() tea.Msg {
		path, err := o.FetchMedia(ctx, chatID, msgID, slot)
		if err != nil {
			return errStatusBackground(action, err)
		}
		img, derr := decodeImageFile(path)
		if derr != nil {
			o.InvalidateMedia(chatID, msgID, slot)
			return nil
		}
		if transform != nil {
			img = transform(img)
		}
		return PhotoReadyMsg{PhotoID: imageID, Image: img}
	}
}

func fetchPhotoCmd(ctx context.Context, o Owner, chatID int64, msgID int, photoID int64) tea.Cmd {
	return fetchInlineImageCmd(ctx, o, chatID, msgID, domain.PhotoThumb, photoID, "photo download", nil)
}

func fetchVideoThumbCmd(ctx context.Context, o Owner, chatID int64, msgID int, docID int64, crop bool) tea.Cmd {
	var transform func(image.Image) image.Image
	if crop {
		transform = media.CircleCrop // round video note -> circle
	}
	return fetchInlineImageCmd(ctx, o, chatID, msgID, domain.DocThumb, docID, "video thumb download", transform)
}

func fetchStickerCmd(ctx context.Context, o Owner, chatID int64, msgID int, docID int64) tea.Cmd {
	return fetchInlineImageCmd(ctx, o, chatID, msgID, domain.DocFull, docID, "sticker download", nil)
}

// fetchAvatarCmd fetches, decodes and hands over a person's avatar. It is a
// separate path from the inline-image one and shares nothing with it but the
// decode: an avatar is named by a person rather than by a message slot, and it
// is cropped to a circle because that is what an avatar looks like everywhere
// else in Telegram (#223).
//
// A failure leaves the monogram on screen and says nothing: the person opened a
// profile to read a name and a bio, and a line about a picture that did not
// arrive is noise. The background status carries it for anyone looking.
func fetchAvatarCmd(ctx context.Context, o Owner, userID, avatarID int64) tea.Cmd {
	return func() tea.Msg {
		path, err := o.FetchAvatar(ctx, userID, avatarID)
		if err != nil {
			return errStatusBackground("avatar download", err)
		}
		img, derr := decodeImageFile(path)
		if derr != nil {
			o.InvalidateAvatar(userID, avatarID)
			return nil
		}
		return avatarReadyMsg{userID: userID, avatarID: avatarID, img: media.CircleCrop(img)}
	}
}

// fetchVoiceCmd fetches a voice note and hands its bytes to the player. Voice
// notes are a few tens of kilobytes, so reading the whole file is fine.
func fetchVoiceCmd(ctx context.Context, o Owner, chatID int64, msgID int, docID int64) tea.Cmd {
	return func() tea.Msg {
		path, err := o.FetchMedia(ctx, chatID, msgID, domain.DocFull)
		if err != nil {
			return errStatus("voice download", err)
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil || len(data) == 0 {
			o.InvalidateMedia(chatID, msgID, domain.DocFull)
			return nil
		}
		return voicePlayReadyMsg{docID: docID, data: data}
	}
}

// handleDownloadSelected saves the selected message's media to the Downloads
// folder. It covers any downloadable document-backed media (video, round note,
// voice, audio, GIF, generic file) and photos. No-op when nothing downloadable
// is selected.
func (m RootModel) handleDownloadSelected() (RootModel, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	msgID := m.chat.SelectedMessageID()
	if _, _, ok := m.chat.SelectedMessageDownloadDoc(); ok {
		return m.startFileDownload(msgID)
	}
	if _, ok := m.chat.SelectedMessagePhoto(); ok {
		return m.startPhotoDownload(msgID)
	}
	return m, nil
}

// openPhotoExternal opens the selected photo in the OS default image viewer
// using the cached (full-quality when available) image. No-op if not cached.
func (m RootModel) openPhotoExternal(photoID int64) (RootModel, tea.Cmd) {
	img, ok := m.fullImageCache.Get(photoID)
	if !ok {
		img, _ = m.imageCache.Get(photoID)
	}
	if img != nil {
		go openInViewer(img, m.tmpDir)
	}
	return m, nil
}

// startDocumentOpen sets the status-bar download indicator with label and
// dispatches the external-player download; the completion message clears the
// matching indicator (and surfaces any error).
func (m RootModel) startDocumentOpen(msgID int, label string) (RootModel, tea.Cmd) {
	serial := m.statusBar.StartDownload(label)
	return m, openDocumentCmd(m.ctx, m.owner, m.currentChatID, msgID, m.tmpDir, serial)
}

// selectedDownloadLabel returns the download indicator label for the selected
// media, naming the kind. The owner picks the file name now, so the client no
// longer has one to show and says what sort of thing is coming instead.
func (m RootModel) selectedDownloadLabel() string {
	noun := "file"
	if m.st != nil && m.chat != nil {
		id := m.chat.SelectedMessageID()
		for _, msg := range m.st.Messages(m.currentChatID) {
			if msg.ID != id {
				continue
			}
			if msg.Media != nil {
				switch msg.Media.Kind {
				case domain.MediaVideo:
					noun = "video"
				case domain.MediaVideoNote:
					noun = "note"
				case domain.MediaVoice:
					noun = "voice message"
				case domain.MediaAudio:
					noun = "audio"
				case domain.MediaGIF:
					noun = "GIF"
				}
			}
			break
		}
	}
	return "downloading " + noun + "…"
}

// startFileDownload sets the status-bar download indicator and dispatches a
// streaming download of a generic file to the Downloads folder. The owner names
// the file, so the indicator names the media kind instead of the file name.
func (m RootModel) startFileDownload(msgID int) (RootModel, tea.Cmd) {
	serial := m.statusBar.StartDownload(m.selectedDownloadLabel())
	return m, saveFileCmd(m.ctx, m.owner, m.currentChatID, msgID, domain.DocFull, downloadsDir(), serial)
}

// startPhotoDownload sets the status-bar download indicator and dispatches a
// streaming download of a photo (full quality) to the Downloads folder.
func (m RootModel) startPhotoDownload(msgID int) (RootModel, tea.Cmd) {
	serial := m.statusBar.StartDownload("downloading photo…")
	return m, saveFileCmd(m.ctx, m.owner, m.currentChatID, msgID, domain.PhotoFull, downloadsDir(), serial)
}

// saveFileCmd streams the named media into destDir under the name the owner
// picks, and reports the saved path (or the error).
func saveFileCmd(ctx context.Context, o Owner, chatID int64, msgID int, slot domain.MediaSlot, destDir string, serial int) tea.Cmd {
	return func() tea.Msg {
		path, err := o.SaveMedia(ctx, chatID, msgID, slot, destDir)
		if err != nil {
			text, sev, _ := errText("download", err)
			return fileDownloadDoneMsg{serial: serial, text: text, sev: sev}
		}
		return fileDownloadDoneMsg{serial: serial, text: "Saved to " + path, sev: components.SeverityInfo}
	}
}

// openDocumentCmd saves a document into tmpDir and opens it in the OS default
// application (e.g. a video player). Runs async; the download may be large. It
// always returns a documentOpenDoneMsg so the caller can clear the status-bar
// download indicator identified by serial (and surface any error).
func openDocumentCmd(ctx context.Context, o Owner, chatID int64, msgID int, tmpDir string, serial int) tea.Cmd {
	return func() tea.Msg {
		path, err := o.SaveMedia(ctx, chatID, msgID, domain.DocFull, tmpDir)
		if err != nil {
			text, sev, _ := errText("open file", err)
			return documentOpenDoneMsg{serial: serial, errText: text, sev: sev}
		}
		openPath(path)
		return documentOpenDoneMsg{serial: serial}
	}
}

// DocumentOpenErrTextForTest reports whether msg is a documentOpenDoneMsg and,
// if so, its error text ("" on success).
func DocumentOpenErrTextForTest(msg tea.Msg) (string, bool) {
	if d, ok := msg.(documentOpenDoneMsg); ok {
		return d.errText, true
	}
	return "", false
}

// DocumentOpenDoneMsgForTest builds a documentOpenDoneMsg for tests.
func DocumentOpenDoneMsgForTest(serial int, errText string, sev components.Severity) tea.Msg {
	return documentOpenDoneMsg{serial: serial, errText: errText, sev: sev}
}

// FileDownloadDoneTextForTest reports whether msg is a fileDownloadDoneMsg and,
// if so, its text and severity.
func FileDownloadDoneTextForTest(msg tea.Msg) (string, components.Severity, bool) {
	if d, ok := msg.(fileDownloadDoneMsg); ok {
		return d.text, d.sev, true
	}
	return "", 0, false
}

// FileDownloadDoneMsgForTest builds a fileDownloadDoneMsg for tests.
func FileDownloadDoneMsgForTest(serial int, text string, sev components.Severity) tea.Msg {
	return fileDownloadDoneMsg{serial: serial, text: text, sev: sev}
}

// SetDownloadsDirForTest overrides the Downloads directory resolver and returns
// a restore func, so download tests never touch the real Downloads folder.
func SetDownloadsDirForTest(dir string) func() {
	prev := downloadsDir
	downloadsDir = func() string { return dir }
	return func() { downloadsDir = prev }
}

// SetOpenPathForTest swaps the OS file launcher and returns a restore func.
func SetOpenPathForTest(fn func(string)) func() {
	prev := openPath
	openPath = fn
	return func() { openPath = prev }
}

// saveFullPhotoCmd saves the full-quality photo into tmpDir, decodes it for the
// viewer and removes the file: the viewer holds the decoded image, and the
// full-size cache is deliberately in memory only.
//
// quiet marks the eager prefetch, which runs for every photo in the window
// without anyone asking and therefore keeps an expired file reference to
// itself. The viewer passes false: there the user is looking at the photo and
// would otherwise wonder why it stays at preview quality.
func saveFullPhotoCmd(ctx context.Context, o Owner, chatID int64, msgID int, photoID int64, tmpDir string, quiet bool) tea.Cmd {
	return func() tea.Msg {
		path, err := o.SaveMedia(ctx, chatID, msgID, domain.PhotoFull, tmpDir)
		if err != nil {
			if quiet {
				return errStatusBackground("full photo download", err)
			}
			return errStatus("full photo download", err)
		}
		defer func() { _ = os.Remove(path) }()
		img, derr := decodeImageFile(path)
		if derr != nil {
			return nil
		}
		return FullPhotoReadyMsg{PhotoID: photoID, Image: img}
	}
}

func (m RootModel) pendingDownloadCmds(msgs []domain.Message) tea.Cmd {
	if m.owner == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, msg := range msgs {
		if msg.Photo != nil {
			if !m.imageCache.Contains(msg.Photo.ID) {
				cmds = append(cmds, fetchPhotoCmd(m.ctx, m.owner, msg.ChatID, msg.ID, msg.Photo.ID))
			}
			if m.cfg != nil && m.cfg.Photos.EagerFullQuality && msg.Photo.FullThumbSize != "" {
				if !m.fullImageCache.Contains(msg.Photo.ID) {
					cmds = append(cmds, saveFullPhotoCmd(m.ctx, m.owner, msg.ChatID, msg.ID, msg.Photo.ID, m.tmpDir, true))
				}
			}
		}
		// Video and GIF thumbnails reuse the inline-image cache, keyed by document id.
		if msg.Media != nil && (msg.Media.Kind.IsVideo() || msg.Media.Kind == domain.MediaGIF) && msg.Document != nil && msg.Document.ThumbSize != "" {
			if !m.imageCache.Contains(msg.Document.ID) {
				// Round video notes are cropped to a circle, but only in Kitty mode
				// (PNG alpha); block-art has no transparency, so keep it square there.
				crop := msg.Media.Kind == domain.MediaVideoNote && m.imageMode == media.ModeKitty
				cmds = append(cmds, fetchVideoThumbCmd(m.ctx, m.owner, msg.ChatID, msg.ID, msg.Document.ID, crop))
			}
		}
		// Static WEBP stickers render inline (Kitty only); decode the full document.
		if m.imageMode == media.ModeKitty && domain.IsStaticSticker(msg.Media, msg.Document) {
			if !m.imageCache.Contains(msg.Document.ID) {
				cmds = append(cmds, fetchStickerCmd(m.ctx, m.owner, msg.ChatID, msg.ID, msg.Document.ID))
			}
		}
	}
	return tea.Batch(cmds...)
}

// PendingDownloadCmdsForTest exposes pendingDownloadCmds for tests.
func (m RootModel) PendingDownloadCmdsForTest(msgs []domain.Message) tea.Cmd {
	return m.pendingDownloadCmds(msgs)
}
