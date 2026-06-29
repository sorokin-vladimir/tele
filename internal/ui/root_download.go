package ui

import (
	"context"
	"image"
	"io"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/gabriel-vasile/mimetype"

	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// downloadWithRefresh runs download(ref); on a FILE_REFERENCE_EXPIRED error it
// refreshes the message's media refs once via RefreshMessage and retries with the
// fresh ref. On a successful retry it returns the refreshed message so the caller
// can persist the new ref.
func downloadWithRefresh[T any, R any](
	ctx context.Context,
	client internaltg.Client,
	peer store.Peer,
	msgID int,
	ref R,
	download func(R) (T, error),
	pickRef func(store.Message) (R, bool),
) (result T, refreshed *store.Message, err error) {
	result, err = download(ref)
	if err == nil {
		return result, nil, nil
	}
	if !internaltg.IsFileReferenceExpired(err) {
		return result, nil, err
	}
	msg, rerr := client.RefreshMessage(ctx, peer, msgID)
	if rerr != nil {
		return result, nil, err
	}
	newRef, ok := pickRef(msg)
	if !ok {
		return result, nil, err
	}
	result, err = download(newRef)
	if err != nil {
		return result, nil, err
	}
	return result, &msg, nil
}

func downloadPhotoCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.PhotoRef) tea.Cmd {
	return func() tea.Msg {
		img, refreshed, err := downloadWithRefresh(ctx, client, peer, msgID, ref,
			func(r store.PhotoRef) (image.Image, error) {
				return client.DownloadPhoto(ctx, r)
			},
			func(m store.Message) (store.PhotoRef, bool) {
				if m.Photo == nil {
					return store.PhotoRef{}, false
				}
				return *m.Photo, true
			},
		)
		if err != nil {
			return StatusErrMsg{Text: "photo download failed: " + err.Error(), Sev: components.SeverityWarning}
		}
		ready := PhotoReadyMsg{PhotoID: ref.ID, Image: img}
		if refreshed != nil {
			return refreshedBatch(ready, mediaRefRefreshedMsg{chatID: peer.ID, msgID: msgID, photo: refreshed.Photo})
		}
		return ready
	}
}

// DownloadPhotoCmdForTest exposes downloadPhotoCmd for tests.
func DownloadPhotoCmdForTest(c internaltg.Client, peer store.Peer, msgID int, ref store.PhotoRef) tea.Cmd {
	return downloadPhotoCmd(context.Background(), c, peer, msgID, ref)
}

// HistoryChunkMsgForTest builds a historyChunkMsg for tests.
func HistoryChunkMsgForTest(chatID int64, msgs []store.Message) tea.Msg {
	return historyChunkMsg{chatID: chatID, messages: msgs}
}

// refreshedBatch emits both the ready image and the store-update message after a
// successful refresh+retry.
func refreshedBatch(ready, refreshed tea.Msg) tea.Msg {
	return tea.BatchMsg{
		func() tea.Msg { return ready },
		func() tea.Msg { return refreshed },
	}
}

// currentPeer returns the peer of the currently open chat, or the zero peer.
func (m RootModel) currentPeer() store.Peer {
	if m.st != nil {
		if chat, ok := m.st.GetChat(m.currentChatID); ok {
			return chat.Peer
		}
	}
	return store.Peer{}
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
	if ref, kind, ok := m.chat.SelectedMessageDownloadDoc(); ok {
		return m.startFileDownload(ref, kind, msgID)
	}
	if ref, ok := m.chat.SelectedMessagePhoto(); ok {
		return m.startPhotoDownload(ref, msgID)
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
func (m RootModel) startDocumentOpen(ref store.DocumentRef, msgID int, label string) (RootModel, tea.Cmd) {
	serial := m.statusBar.StartDownload(label)
	return m, openDocumentCmd(m.ctx, m.tgClient, m.currentPeer(), msgID, ref, m.tmpDir, serial)
}

// selectedDownloadLabel returns the download indicator label for the selected
// media: round notes read "note", everything else "video".
func (m RootModel) selectedDownloadLabel() string {
	if m.st != nil && m.chat != nil {
		id := m.chat.SelectedMessageID()
		for _, msg := range m.st.Messages(m.currentChatID) {
			if msg.ID == id {
				if msg.Media != nil && msg.Media.Kind == store.MediaVideoNote {
					return "downloading note…"
				}
				break
			}
		}
	}
	return "downloading video…"
}

// startFileDownload sets the status-bar download indicator and dispatches a
// streaming download of a generic file to the Downloads folder.
func (m RootModel) startFileDownload(ref store.DocumentRef, kind store.MediaKind, msgID int) (RootModel, tea.Cmd) {
	name := downloadFileName(ref, kind)
	ref.FileName = name
	serial := m.statusBar.StartDownload("downloading " + name + "…")
	return m, downloadFileCmd(m.ctx, m.tgClient, m.currentPeer(), msgID, ref, downloadsDir(), serial)
}

// startPhotoDownload sets the status-bar download indicator and dispatches a
// streaming download of a photo (full quality) to the Downloads folder.
func (m RootModel) startPhotoDownload(ref store.PhotoRef, msgID int) (RootModel, tea.Cmd) {
	serial := m.statusBar.StartDownload("downloading photo…")
	return m, downloadPhotoFileCmd(m.ctx, m.tgClient, m.currentPeer(), msgID, ref, downloadsDir(), serial)
}

// downloadPhotoFileCmd streams a photo's full-quality bytes to destDir as
// photo_<id>.jpg (collision-resolved) and reports the saved path (or an error).
// Mirrors downloadFileCmd's stream-to-disk + FILE_REFERENCE_EXPIRED retry.
func downloadPhotoFileCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.PhotoRef, destDir string, serial int) tea.Cmd {
	fullRef := ref
	fullRef.ThumbSize = ref.FullThumbSize
	return func() tea.Msg {
		fail := func(text string) tea.Msg {
			return fileDownloadDoneMsg{serial: serial, text: text, sev: components.SeverityWarning}
		}
		f, err := createUniqueDownloadFile(destDir, "photo_"+itoa64(ref.ID)+".jpg")
		if err != nil {
			return fail("download failed: " + err.Error())
		}
		name := f.Name()

		_, refreshed, derr := downloadWithRefresh(ctx, client, peer, msgID, fullRef,
			func(r store.PhotoRef) (struct{}, error) {
				if _, serr := f.Seek(0, io.SeekStart); serr != nil {
					return struct{}{}, serr
				}
				if terr := f.Truncate(0); terr != nil {
					return struct{}{}, terr
				}
				return struct{}{}, client.DownloadPhotoToFile(ctx, r, f)
			},
			func(m store.Message) (store.PhotoRef, bool) {
				if m.Photo == nil {
					return store.PhotoRef{}, false
				}
				r := *m.Photo
				r.ThumbSize = r.FullThumbSize
				return r, true
			},
		)
		if derr != nil {
			_ = f.Close()
			_ = os.Remove(name)
			return fail("download failed: " + derr.Error())
		}
		if cerr := f.Close(); cerr != nil {
			_ = os.Remove(name)
			return fail("download failed: " + cerr.Error())
		}
		done := fileDownloadDoneMsg{serial: serial, text: "Saved to " + name, sev: components.SeverityInfo, chatID: peer.ID, msgID: msgID}
		if refreshed != nil {
			done.photo = refreshed.Photo
		}
		return done
	}
}

// DownloadPhotoFileCmdForTest exposes downloadPhotoFileCmd for tests (serial 0).
func DownloadPhotoFileCmdForTest(c internaltg.Client, peer store.Peer, msgID int, ref store.PhotoRef, destDir string) tea.Cmd {
	return downloadPhotoFileCmd(context.Background(), c, peer, msgID, ref, destDir, 0)
}

// downloadFileCmd streams a document to destDir under its original name
// (collision-resolved) and reports the saved path (or an error). Mirrors
// openDocumentCmd's stream-to-disk + FILE_REFERENCE_EXPIRED retry.
func downloadFileCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef, destDir string, serial int) tea.Cmd {
	return func() tea.Msg {
		fail := func(text string) tea.Msg {
			return fileDownloadDoneMsg{serial: serial, text: text, sev: components.SeverityWarning}
		}
		f, err := createUniqueDownloadFile(destDir, ref.FileName)
		if err != nil {
			return fail("download failed: " + err.Error())
		}
		name := f.Name()

		_, refreshed, derr := downloadWithRefresh(ctx, client, peer, msgID, ref,
			func(r store.DocumentRef) (struct{}, error) {
				if _, serr := f.Seek(0, io.SeekStart); serr != nil {
					return struct{}{}, serr
				}
				if terr := f.Truncate(0); terr != nil {
					return struct{}{}, terr
				}
				return struct{}{}, client.DownloadDocumentToFile(ctx, r, f)
			},
			pickDocumentRef,
		)
		if derr != nil {
			_ = f.Close()
			_ = os.Remove(name)
			return fail("download failed: " + derr.Error())
		}
		if cerr := f.Close(); cerr != nil {
			_ = os.Remove(name)
			return fail("download failed: " + cerr.Error())
		}
		done := fileDownloadDoneMsg{serial: serial, text: "Saved to " + name, sev: components.SeverityInfo, chatID: peer.ID, msgID: msgID}
		if refreshed != nil {
			done.doc = refreshed.Document
		}
		return done
	}
}

// openDocumentCmd downloads a document in full and opens it in the OS default
// application (e.g. a video player). Runs async; the download may be large. It
// always returns a documentOpenDoneMsg so the caller can clear the status-bar
// download indicator identified by serial (and surface any error).
func openDocumentCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef, tmpDir string, serial int) tea.Cmd {
	return func() tea.Msg {
		fail := func(text string) tea.Msg {
			return documentOpenDoneMsg{serial: serial, errText: text, sev: components.SeverityWarning}
		}
		ext := filepath.Ext(ref.FileName)
		if ext == "" {
			ext = extFromMime(ref.MimeType)
		}
		f, err := createTempMediaFile(tmpDir, ext)
		if err != nil {
			return fail("open file failed: " + err.Error())
		}
		name := f.Name()

		// Stream directly to disk; the whole file is never held in memory. On a
		// FILE_REFERENCE_EXPIRED retry the file is truncated so a partial first
		// attempt does not corrupt the result.
		_, refreshed, derr := downloadWithRefresh(ctx, client, peer, msgID, ref,
			func(r store.DocumentRef) (struct{}, error) {
				if _, serr := f.Seek(0, io.SeekStart); serr != nil {
					return struct{}{}, serr
				}
				if terr := f.Truncate(0); terr != nil {
					return struct{}{}, terr
				}
				return struct{}{}, client.DownloadDocumentToFile(ctx, r, f)
			},
			pickDocumentRef,
		)
		if derr != nil {
			_ = f.Close()
			_ = os.Remove(name)
			return fail("open file failed: " + derr.Error())
		}
		if cerr := f.Close(); cerr != nil {
			_ = os.Remove(name)
			return fail("open file failed: " + cerr.Error())
		}
		openPath(name)
		done := documentOpenDoneMsg{serial: serial, chatID: peer.ID, msgID: msgID}
		if refreshed != nil {
			done.doc = refreshed.Document
		}
		return done
	}
}

// OpenDocumentCmdForTest exposes openDocumentCmd for tests (serial 0).
func OpenDocumentCmdForTest(c internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef, tmpDir string) tea.Cmd {
	return openDocumentCmd(context.Background(), c, peer, msgID, ref, tmpDir, 0)
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

// DownloadFileCmdForTest exposes downloadFileCmd for tests (serial 0).
func DownloadFileCmdForTest(c internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef, destDir string) tea.Cmd {
	return downloadFileCmd(context.Background(), c, peer, msgID, ref, destDir, 0)
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

// pickDocumentRef extracts a message's fresh document ref, used as the refresh
// selector for document downloads.
func pickDocumentRef(m store.Message) (store.DocumentRef, bool) {
	if m.Document == nil {
		return store.DocumentRef{}, false
	}
	return *m.Document, true
}

// downloadFileName returns the on-disk name for a downloaded document: the
// original FileName when present, otherwise a synthesized "<prefix>_<id><ext>"
// where the prefix reflects the media kind and the extension is derived from the
// MIME type. Telegram photos/voice/round notes often carry no file name.
func downloadFileName(ref store.DocumentRef, kind store.MediaKind) string {
	if ref.FileName != "" {
		return ref.FileName
	}
	prefix := "file"
	switch kind {
	case store.MediaVideo:
		prefix = "video"
	case store.MediaVideoNote:
		prefix = "video_note"
	case store.MediaVoice:
		prefix = "voice"
	case store.MediaAudio:
		prefix = "audio"
	case store.MediaGIF:
		prefix = "gif"
	}
	// Derive the extension from the MIME type via the mimetype library rather
	// than a hand-rolled table; unknown types yield no extension.
	ext := ""
	if mt := mimetype.Lookup(ref.MimeType); mt != nil {
		ext = mt.Extension()
	}
	return prefix + "_" + itoa64(ref.ID) + ext
}

// extFromMime maps common video MIME types to a file extension so the OS picks
// the right player. Defaults to .mp4 (the usual Telegram video container).
func extFromMime(mime string) string {
	switch mime {
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "video/x-matroska":
		return ".mkv"
	default:
		return ".mp4"
	}
}

func downloadVoiceCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef) tea.Cmd {
	return func() tea.Msg {
		data, refreshed, err := downloadWithRefresh(ctx, client, peer, msgID, ref,
			func(r store.DocumentRef) ([]byte, error) {
				return client.DownloadDocument(ctx, r)
			},
			pickDocumentRef,
		)
		if err != nil {
			return StatusErrMsg{Text: "voice download failed: " + err.Error(), Sev: components.SeverityWarning}
		}
		if len(data) == 0 {
			return nil
		}
		ready := voicePlayReadyMsg{docID: ref.ID, data: data}
		if refreshed != nil {
			return refreshedBatch(ready, mediaRefRefreshedMsg{chatID: peer.ID, msgID: msgID, doc: refreshed.Document})
		}
		return ready
	}
}

func downloadVideoThumbCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef, crop bool) tea.Cmd {
	return func() tea.Msg {
		img, refreshed, err := downloadWithRefresh(ctx, client, peer, msgID, ref,
			func(r store.DocumentRef) (image.Image, error) {
				return client.DownloadDocumentThumb(ctx, r)
			},
			func(m store.Message) (store.DocumentRef, bool) {
				if m.Document == nil {
					return store.DocumentRef{}, false
				}
				return *m.Document, true
			},
		)
		if err != nil || img == nil {
			if err != nil {
				return StatusErrMsg{Text: "video thumb download failed: " + err.Error(), Sev: components.SeverityWarning}
			}
			return nil
		}
		if crop {
			img = media.CircleCrop(img) // round video note → circle
		}
		// Reuse the photo-ready path; the cache is keyed by id (here the document id).
		ready := PhotoReadyMsg{PhotoID: ref.ID, Image: img}
		if refreshed != nil {
			return refreshedBatch(ready, mediaRefRefreshedMsg{chatID: peer.ID, msgID: msgID, doc: refreshed.Document})
		}
		return ready
	}
}

// downloadStickerCmd downloads and decodes a static WEBP sticker (the full
// document) and feeds it through the inline-image cache keyed by document id.
func downloadStickerCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef) tea.Cmd {
	return func() tea.Msg {
		img, refreshed, err := downloadWithRefresh(ctx, client, peer, msgID, ref,
			func(r store.DocumentRef) (image.Image, error) {
				return client.DownloadDocumentImage(ctx, r)
			},
			func(m store.Message) (store.DocumentRef, bool) {
				if m.Document == nil {
					return store.DocumentRef{}, false
				}
				return *m.Document, true
			},
		)
		if err != nil || img == nil {
			if err != nil {
				return StatusErrMsg{Text: "sticker download failed: " + err.Error(), Sev: components.SeverityWarning}
			}
			return nil
		}
		ready := PhotoReadyMsg{PhotoID: ref.ID, Image: img}
		if refreshed != nil {
			return refreshedBatch(ready, mediaRefRefreshedMsg{chatID: peer.ID, msgID: msgID, doc: refreshed.Document})
		}
		return ready
	}
}

// DownloadStickerCmdForTest exposes downloadStickerCmd for tests.
func DownloadStickerCmdForTest(c internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef) tea.Cmd {
	return downloadStickerCmd(context.Background(), c, peer, msgID, ref)
}

func downloadFullPhotoCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.PhotoRef) tea.Cmd {
	fullRef := ref
	fullRef.ThumbSize = ref.FullThumbSize
	return func() tea.Msg {
		img, refreshed, err := downloadWithRefresh(ctx, client, peer, msgID, fullRef,
			func(r store.PhotoRef) (image.Image, error) {
				return client.DownloadPhoto(ctx, r)
			},
			func(m store.Message) (store.PhotoRef, bool) {
				if m.Photo == nil {
					return store.PhotoRef{}, false
				}
				r := *m.Photo
				r.ThumbSize = r.FullThumbSize
				return r, true
			},
		)
		if err != nil || img == nil {
			if err != nil {
				return StatusErrMsg{Text: "full photo download failed: " + err.Error(), Sev: components.SeverityWarning}
			}
			return nil
		}
		ready := FullPhotoReadyMsg{PhotoID: ref.ID, Image: img}
		if refreshed != nil {
			return refreshedBatch(ready, mediaRefRefreshedMsg{chatID: peer.ID, msgID: msgID, photo: refreshed.Photo})
		}
		return ready
	}
}

func (m RootModel) pendingDownloadCmds(msgs []store.Message) tea.Cmd {
	var cmds []tea.Cmd
	for _, msg := range msgs {
		var peer store.Peer
		if m.st != nil {
			if chat, ok := m.st.GetChat(msg.ChatID); ok {
				peer = chat.Peer
			}
		}
		if msg.Photo != nil {
			if !m.imageCache.Contains(msg.Photo.ID) {
				cmds = append(cmds, downloadPhotoCmd(m.ctx, m.tgClient, peer, msg.ID, *msg.Photo))
			}
			if m.cfg != nil && m.cfg.Photos.EagerFullQuality && msg.Photo.FullThumbSize != "" {
				if !m.fullImageCache.Contains(msg.Photo.ID) {
					cmds = append(cmds, downloadFullPhotoCmd(m.ctx, m.tgClient, peer, msg.ID, *msg.Photo))
				}
			}
		}
		// Video and GIF thumbnails reuse the inline-image cache, keyed by document id.
		if msg.Media != nil && (msg.Media.Kind.IsVideo() || msg.Media.Kind == store.MediaGIF) && msg.Document != nil && msg.Document.ThumbSize != "" {
			if !m.imageCache.Contains(msg.Document.ID) {
				// Round video notes are cropped to a circle, but only in Kitty mode
				// (PNG alpha); block-art has no transparency, so keep it square there.
				crop := msg.Media.Kind == store.MediaVideoNote && m.imageMode == media.ModeKitty
				cmds = append(cmds, downloadVideoThumbCmd(m.ctx, m.tgClient, peer, msg.ID, *msg.Document, crop))
			}
		}
		// Static WEBP stickers render inline (Kitty only); decode the full document.
		if m.imageMode == media.ModeKitty && store.IsStaticSticker(msg.Media, msg.Document) {
			if !m.imageCache.Contains(msg.Document.ID) {
				cmds = append(cmds, downloadStickerCmd(m.ctx, m.tgClient, peer, msg.ID, *msg.Document))
			}
		}
	}
	return tea.Batch(cmds...)
}

// PendingDownloadCmdsForTest exposes pendingDownloadCmds for tests.
func (m RootModel) PendingDownloadCmdsForTest(msgs []store.Message) tea.Cmd {
	return m.pendingDownloadCmds(msgs)
}
