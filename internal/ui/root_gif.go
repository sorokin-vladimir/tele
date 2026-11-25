package ui

import (
	"context"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	vmedia "github.com/sorokin-vladimir/tele/internal/media"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// gifFPS is the inline GIF playback rate. gifFrameInterval is the tick period.
const gifFPS = 12

var gifFrameInterval = time.Second / gifFPS

// gifMaxFrames bounds decode memory for a single looping GIF.
const gifMaxFrames = 300

// gifSpinnerFrames is the braille spinner shown in a GIF badge while it downloads
// and decodes.
var gifSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// updateGifLoadingSpinner advances the loading-spinner glyph and tells the chat
// which GIF (if any) is still being fetched. A GIF is "loading" while it is the
// active selection but its frames are not decoded yet. Driven off the existing
// SpinnerTickMsg cadence — no dedicated ticker.
func (m *RootModel) updateGifLoadingSpinner() {
	if m.chat == nil {
		return
	}
	loadingID := int64(0)
	if m.gifActiveID != 0 && len(m.gifFrames[m.gifActiveID]) == 0 {
		loadingID = m.gifActiveID
	}
	if loadingID == 0 {
		m.chat.SetGifLoading(0, "")
		return
	}
	m.gifSpinnerIdx++
	glyph := gifSpinnerFrames[m.gifSpinnerIdx%len(gifSpinnerFrames)]
	m.chat.SetGifLoading(loadingID, glyph)
}

// saveGifFileCmd saves a GIF's full MP4 into tmpDir and reports its path so it
// can be decoded into frames. It mirrors openDocumentCmd but yields the path
// instead of launching an external player.
func saveGifFileCmd(ctx context.Context, o Owner, chatID int64, msgID int, docID int64, tmpDir string) tea.Cmd {
	return func() tea.Msg {
		path, err := o.SaveMedia(ctx, chatID, msgID, domain.DocFull, tmpDir, "")
		if err != nil {
			return nil
		}
		return gifFileReadyMsg{docID: docID, msgID: msgID, path: path}
	}
}

// decodeGifCmd decodes a downloaded GIF file into frames sized to w×h px.
func decodeGifCmd(ctx context.Context, docID int64, path string, w, h int) tea.Cmd {
	return func() tea.Msg {
		// Frames are held in memory after decoding, so the temp file is no longer
		// needed; remove it regardless of success to avoid piling up on disk.
		defer func() { _ = os.Remove(path) }()
		frames, err := vmedia.DecodeAllFrames(ctx, path, w, h, gifFPS, gifMaxFrames)
		if err != nil || len(frames) == 0 {
			return nil
		}
		return gifFramesReadyMsg{docID: docID, frames: frames}
	}
}

// gifTickCmd schedules the next animation frame, tagged with the current gen.
func gifTickCmd(gen int) tea.Cmd {
	return tea.Tick(gifFrameInterval, func(time.Time) tea.Msg {
		return gifTickMsg{gen: gen}
	})
}

// stopGifAnim halts the active animation, resets the active gif to its first
// frame (so it doesn't freeze mid-loop), and bumps the generation so any
// in-flight tick is ignored.
func (m *RootModel) stopGifAnim() {
	if m.gifActiveID != 0 {
		if frames := m.gifFrames[m.gifActiveID]; len(frames) > 0 {
			m.imageCache.Add(m.gifActiveID, frames[0])
		}
	}
	m.gifActiveID = 0
	m.gifIdx = 0
	m.gifGen++
	// A frame still on its way belongs to the GIF stopped: the next one to
	// start must not wait for it.
	m.gifInFlight = false
}

// reconcileGifAnim is called whenever the selected message changes. It stops any
// running animation and, if the new selection is a GIF (Kitty + ffmpeg), starts
// (or kicks off downloading/decoding) its loop.
func (m RootModel) reconcileGifAnim() (RootModel, tea.Cmd) {
	m.stopGifAnim()

	if m.imageMode != media.ModeKitty || !vmedia.HasFFmpeg() || m.chat == nil {
		return m, nil
	}
	ref, ok := m.chat.SelectedMessageGIF()
	if !ok {
		return m, nil
	}
	// Need the static thumbnail cached to size the decode (and it confirms the
	// placement exists). If it isn't loaded yet, leave the gif static for now.
	if !m.imageCache.Contains(ref.ID) {
		return m, nil
	}

	// Already decoded: start looping immediately.
	if frames, ok := m.gifFrames[ref.ID]; ok && len(frames) > 0 {
		m.gifActiveID = ref.ID
		m.gifIdx = 0
		m.gifGen++
		return m, gifTickCmd(m.gifGen)
	}

	// Otherwise download the full file, then decode (handled by the msg chain).
	m.gifActiveID = ref.ID
	m.gifGen++
	return m, saveGifFileCmd(m.ctx, m.owner, m.currentChatID, m.chat.SelectedMessageID(), ref.ID, m.tmpDir)
}

// ensureGifAnimForSelection starts the GIF animation for the currently-selected
// message when it is a GIF that isn't already active. Unlike reconcileGifAnim it
// is idempotent for the active gif, so it is safe to call when a chat opens or a
// thumbnail arrives — cases where the newest GIF is selected by default but no key
// event fired. Returns a no-op when nothing needs to start.
func (m RootModel) ensureGifAnimForSelection() (RootModel, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	ref, ok := m.chat.SelectedMessageGIF()
	if !ok || m.gifActiveID == ref.ID {
		return m, nil
	}
	return m.reconcileGifAnim()
}

// handleGifFileReady decodes a freshly downloaded GIF at the cached thumbnail's
// pixel size, unless the selection has already moved on.
func (m RootModel) handleGifFileReady(msg gifFileReadyMsg) (RootModel, tea.Cmd) {
	if m.gifActiveID != msg.docID {
		_ = os.Remove(msg.path) // selection moved on; drop the temp file
		return m, nil
	}
	thumb, ok := m.imageCache.Get(msg.docID)
	if !ok {
		_ = os.Remove(msg.path)
		return m, nil
	}
	b := thumb.Bounds()
	return m, decodeGifCmd(m.ctx, msg.docID, msg.path, b.Dx(), b.Dy())
}

// handleGifFramesReady caches decoded frames and, if the gif is still the active
// selection, starts the loop.
func (m RootModel) handleGifFramesReady(msg gifFramesReadyMsg) (RootModel, tea.Cmd) {
	m.gifFrames[msg.docID] = msg.frames
	if m.gifActiveID != msg.docID {
		return m, nil
	}
	m.gifIdx = 0
	m.gifGen++
	return m, gifTickCmd(m.gifGen)
}

// handleGifTick shows the frame written behind on the last tick, sends the
// next one behind, and re-arms. A frame is never written under the id on
// screen: Ghostty drops the image under an id as soon as new data for it
// arrives and draws nothing until that data is decoded, which blanked the GIF
// at every frame (#331). Stale ticks (older gen) are ignored.
func (m RootModel) handleGifTick(msg gifTickMsg) (RootModel, tea.Cmd) {
	if msg.gen != m.gifGen || m.gifActiveID == 0 {
		return m, nil
	}
	frames := m.gifFrames[m.gifActiveID]
	if len(frames) == 0 {
		return m, nil
	}
	// The terminal is given a frame's time after the write to decode it.
	if m.kittyStore.BackReady(m.gifActiveID) && m.gifBehind != nil &&
		m.clock().Sub(m.gifWrittenAt) >= gifFrameInterval {
		m.kittyStore.Swap(m.gifActiveID)
		m.imageCache.Add(m.gifActiveID, m.gifBehind)
	}
	m.gifIdx = (m.gifIdx + 1) % len(frames)
	// One frame at a time, as in the video modal: while the back id is taken
	// this frame is dropped, and the GIF keeps time (#331).
	if m.gifInFlight || m.kittyStore.BackReady(m.gifActiveID) {
		return m, gifTickCmd(m.gifGen)
	}
	frame := frames[m.gifIdx]
	m.gifInFlight = true
	m.gifBehind = frame
	transmit := m.transmitFrameBehindCmd(m.gifActiveID, frame)
	return m, tea.Batch(transmit, gifTickCmd(m.gifGen))
}

// GifFileReadyForTest runs saveGifFileCmd and returns the resulting document id
// and saved path (ok=false if it did not produce a gifFileReadyMsg).
func GifFileReadyForTest(o Owner, chatID int64, msgID int, docID int64, tmpDir string) (int64, string, bool) {
	msg := saveGifFileCmd(context.Background(), o, chatID, msgID, docID, tmpDir)()
	r, ok := msg.(gifFileReadyMsg)
	if !ok {
		return 0, "", false
	}
	return r.docID, r.path, true
}
