package ui

import (
	"image"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// widthRepaintMsg follows the renderer clear after Unicode width negotiation.
type widthRepaintMsg struct{}

type PhotoReadyMsg struct {
	PhotoID int64
	Image   image.Image
}

// inlineSixelTickMsg asks the model to paint the inline message images that the
// last frame left dirty. It carries no payload: the frame itself is the state.
type inlineSixelTickMsg struct{}

type inlineSixelClearedMsg struct{ state *inlineSixelState }

type inlineSixelEncodedMsg struct {
	state    *inlineSixelState
	revision uint64
	regions  []inlineRegion
	seq      string
}

// gifFileReadyMsg carries the temp-file path of a downloaded GIF (the full MP4),
// ready to be decoded into frames.
type gifFileReadyMsg struct {
	docID int64
	msgID int
	path  string
}

// gifFramesReadyMsg carries the decoded frames of a GIF, ready to cache and loop.
type gifFramesReadyMsg struct {
	docID  int64
	frames []image.Image
}

// gifTickMsg advances the active GIF animation by one frame. gen guards against
// stale ticks from a previous selection.
type gifTickMsg struct {
	gen int
}

// videoFileReadyMsg carries the temp-file path of a downloaded video, ready for
// the in-app player to decode and display.
type videoFileReadyMsg struct {
	docID int64
	path  string
}

// videoProbedMsg carries a video's real pixel dimensions (from ffprobe) so the
// modal box can be sized to the true aspect before decoding.
type videoProbedMsg struct {
	docID int64
	path  string
	w, h  int
}

// videoTickMsg advances the modal video by one frame. gen drops stale ticks.
type videoTickMsg struct{ gen int }

type FullPhotoReadyMsg struct {
	PhotoID int64
	Image   image.Image
}

// kittyEncodedMsg reports that a photo's Kitty placement was encoded
// successfully and carries the sequence to write to the terminal. It is emitted
// only on encode success, so a failed encode never advances to marking the image
// ready (which would leave the cell blank while the store falsely reports Ready).
// See issue #95.
type kittyEncodedMsg struct {
	photoID int64
	cols    int
	seq     string
}

// kittyTransmittedMsg is emitted after a photo's Kitty virtual placement has
// been written to the terminal. Only then is the image marked ready, so the
// placeholder grid is never painted before the placement exists.
type kittyTransmittedMsg struct {
	photoID int64
	cols    int
}

// sixelEncodedMsg carries a photo modal's image encoded as a sixel sequence.
// key is the store key the encode was started for (photoPlayerKey for the
// modal); cols/rows are the geometry it was encoded at, so a result that
// outlived its box can be dropped instead of painted.
type sixelEncodedMsg struct {
	gen  uint64
	key  int64
	cols int
	rows int
	seq  string
}

// sixelPaintMsg asks for the waiting sixel paint to run now that its frame is
// on screen. gen drops ticks superseded by a newer image or geometry.
type sixelPaintMsg struct{ gen uint64 }

// voicePlayReadyMsg carries a downloaded voice file ready to be played.
type voicePlayReadyMsg struct {
	docID int64
	data  []byte
}

// voiceTickMsg drives the voice playback position/playhead updates.
type voiceTickMsg struct{}

// retransmitTickMsg fires after the photo-width debounce window. Only the tick
// whose gen matches the latest scheduled one performs the retransmit; earlier
// ticks were superseded by a newer width change.
type retransmitTickMsg struct {
	gen int
}

type clearTypingMsg struct{ serial int }

// msgHighlightFadeMsg advances the jump-to message-bubble highlight fade by one
// step. serial guards against stale ticks from a superseded highlight.
type msgHighlightFadeMsg struct{ serial int }

// chatHighlightFadeMsg advances the chat-list row highlight fade by one step.
// serial guards against stale ticks from a superseded highlight.
type chatHighlightFadeMsg struct{ serial int }

// StatusErrMsg surfaces a transient, severity-tagged error in the status bar.
type StatusErrMsg struct {
	Text string
	Sev  components.Severity
}

// ClearStatusErrMsg clears the status-bar error identified by Serial.
type ClearStatusErrMsg struct{ Serial int }

// documentOpenDoneMsg reports completion of an external-player document open
// started via startDocumentOpen. serial identifies the status-bar download
// indicator to clear. errText is empty on success; on failure it carries the
// error text and sev its severity.
type documentOpenDoneMsg struct {
	serial  int
	errText string
	sev     components.Severity
}

// fileDownloadDoneMsg reports completion of a file download started via
// startFileDownload. serial identifies the status-bar download indicator to
// clear. text is the "Saved to <path>" confirmation on success or the error
// text on failure, with sev distinguishing them.
type fileDownloadDoneMsg struct {
	serial int
	text   string
	sev    components.Severity
}

// chatLoadErrMsg reports a failed chat-open history load.
type chatLoadErrMsg struct {
	chatID int64
	text   string
}
