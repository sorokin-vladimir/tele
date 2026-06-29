package ui

import (
	"image"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

type ChatHistoryMsg struct {
	ChatID   int64
	Messages []store.Message
}

type PhotoReadyMsg struct {
	PhotoID int64
	Image   image.Image
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

// kittyTransmittedMsg is emitted after a photo's Kitty virtual placement has
// been written to the terminal. Only then is the image marked ready, so the
// placeholder grid is never painted before the placement exists.
type kittyTransmittedMsg struct {
	photoID int64
	cols    int
}

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

type markReadDoneMsg struct {
	chatID int64
	maxID  int
}

type historyChunkMsg struct {
	chatID   int64
	messages []store.Message
	err      error
}

type FolderFiltersMsg struct {
	Filters []store.FolderFilter
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
// error text and sev its severity. doc is a refreshed ref (or nil).
type documentOpenDoneMsg struct {
	serial  int
	errText string
	sev     components.Severity
	chatID  int64
	msgID   int
	doc     *store.DocumentRef
}

// fileDownloadDoneMsg reports completion of a file download started via
// startFileDownload. serial identifies the status-bar download indicator to
// clear. text is the "Saved to <path>" confirmation on success or the error
// text on failure, with sev distinguishing them. doc is a refreshed ref (or nil).
type fileDownloadDoneMsg struct {
	serial int
	text   string
	sev    components.Severity
	chatID int64
	msgID  int
	doc    *store.DocumentRef
	photo  *store.PhotoRef
}

// chatLoadErrMsg reports a failed chat-open history load.
type chatLoadErrMsg struct {
	chatID int64
	text   string
}

// mediaRefRefreshedMsg carries refreshed media refs after a FILE_REFERENCE_EXPIRED,
// so the store can keep the fresh refs for subsequent opens.
type mediaRefRefreshedMsg struct {
	chatID int64
	msgID  int
	photo  *store.PhotoRef
	doc    *store.DocumentRef
}
