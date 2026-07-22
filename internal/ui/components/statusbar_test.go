package components_test

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
)

// strip removes ANSI styling so assertions can match the visible text even
// when a hint's key letter is colored mid-word.
func strip(s string) string { return xansi.Strip(s) }

func TestStatusBar_NormalMode(t *testing.T) {
	sb := components.NewStatusBar(80)
	sb.SetMode(keys.ModeNormal)
	assert.Contains(t, strip(sb.View()), "NORMAL")
}

func TestStatusBar_InsertMode(t *testing.T) {
	sb := components.NewStatusBar(80)
	sb.SetMode(keys.ModeInsert)
	assert.Contains(t, strip(sb.View()), "INSERT")
}

func TestStatusBar_StatusText(t *testing.T) {
	sb := components.NewStatusBar(80)
	sb.SetStatus("Loading...")
	assert.Contains(t, strip(sb.View()), "Loading...")
}

func TestStatusBar_ChatHintsIncludeUpload(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(160)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetMode(keys.ModeNormal)
	// "u" is bound to attach and is the first letter of "upload", so the hint
	// highlights the letter in place rather than prefixing the key.
	assert.Contains(t, strip(sb.View()), "upload")
}

func TestStatusBar_ChatHintsIncludeCopy(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(160)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetMode(keys.ModeNormal)
	assert.Contains(t, strip(sb.View()), "copy")
}

func TestStatusBar_ChatHints_WordingFromLabels(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(200)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetMode(keys.ModeNormal)
	out := strip(sb.View())
	// down in chat is labeled "scroll" (not "move") via context override.
	assert.Contains(t, out, "scroll")
	assert.Contains(t, out, "write")
	assert.Contains(t, out, "upload")
	assert.Contains(t, out, "copy")
}

func TestStatusBar_ChatListHints_MoveOpenSearchQuit(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(200)
	sb.SetKeyMap(km)
	sb.SetActivePane("chatlist")
	sb.SetMode(keys.ModeNormal)
	out := strip(sb.View())
	assert.Contains(t, out, "move")
	assert.Contains(t, out, "open")
	assert.Contains(t, out, "search")
	assert.Contains(t, out, "quit")
}

func TestStatusBar_ComposerHints_SendNormal(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(200)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetMode(keys.ModeInsert)
	out := strip(sb.View())
	assert.Contains(t, out, "send")
	assert.Contains(t, out, "normal")
	assert.Contains(t, out, "ctrl+v paste image")
}

func TestStatusBar_AttachStagedNormal_CaptionDrop(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(200)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetMode(keys.ModeNormal)
	sb.SetAttachStaged(true)
	out := strip(sb.View())
	assert.Contains(t, out, "caption")
	assert.Contains(t, out, "drop file")
}

func TestStatusBar_PickerHints(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(160)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetPickerOpen(true)
	view := strip(sb.View())
	assert.Contains(t, view, "type filter")
	assert.Contains(t, view, "open/select ↵")
}

func TestStatusBar_StagedAttachmentHints(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(160)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetAttachStaged(true)

	sb.SetMode(keys.ModeInsert)
	insertView := strip(sb.View())
	assert.Contains(t, insertView, "send ↵")
	assert.Contains(t, insertView, "ctrl+t photo/file")

	sb.SetMode(keys.ModeNormal)
	assert.Contains(t, strip(sb.View()), "x drop file")
}

func TestStatusBar_ChatlistHints(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(120)
	sb.SetKeyMap(km)
	sb.SetActivePane("chatlist")
	view := strip(sb.View())
	assert.Contains(t, view, "j/k move")
	assert.Contains(t, view, "open ↵")
	assert.Contains(t, view, "/ search")
	assert.Contains(t, view, "quit")
	assert.NotContains(t, view, "->")
}

func TestStatusBar_ChatNormalHints(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(120)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetMode(keys.ModeNormal)
	view := strip(sb.View())
	assert.Contains(t, view, "↑ scroll ↓")
	assert.Contains(t, view, "j/k select")
	assert.NotContains(t, view, "menu")
	assert.Contains(t, view, "a write")
	assert.Contains(t, view, "quit")
	assert.NotContains(t, view, "->")
}

func TestStatusBar_ChatInsertHints(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(120)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetMode(keys.ModeInsert)
	view := strip(sb.View())
	assert.Contains(t, view, "send ↵")
	assert.Contains(t, view, "esc normal")
}

func TestStatusBar_NoKeyMap_NoHints(t *testing.T) {
	sb := components.NewStatusBar(80)
	sb.SetActivePane("chatlist")
	view := strip(sb.View())
	assert.NotContains(t, view, "->")
}

func TestStatusBar_SeparatesSegments(t *testing.T) {
	sb := components.NewStatusBar(120)
	sb.SetKeyMap(keys.DefaultKeyMap())
	sb.SetActivePane("chatlist")
	sb.SetStatus("network down")
	view := strip(sb.View())
	assert.Contains(t, view, "network down")
	assert.Contains(t, view, "│") // segment separator present
}

func TestStatusBar_HintsAppendedAfterStatus(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(120)
	sb.SetKeyMap(km)
	sb.SetActivePane("chatlist")
	sb.SetStatus("12 chats")
	view := strip(sb.View())
	statusIdx := strings.Index(view, "12 chats")
	hintIdx := strings.Index(view, "j/k move")
	assert.Greater(t, hintIdx, statusIdx, "hints should appear after status text")
}

func TestStatusBar_StartDownload_ShowsLabelAndSpinner(t *testing.T) {
	sb := components.NewStatusBar(80)
	sb.StartDownload("downloading video…")
	view := strip(sb.View())
	assert.Contains(t, view, "downloading video…")
	assert.Contains(t, view, "[=") // ping-pong spinner frame present
}

func TestStatusBar_ClearDownload_CurrentSerialClears(t *testing.T) {
	sb := components.NewStatusBar(80)
	serial := sb.StartDownload("downloading video…")
	sb.ClearDownload(serial)
	assert.NotContains(t, strip(sb.View()), "downloading video…")
}

func TestStatusBar_ClearDownload_StaleSerialIsNoop(t *testing.T) {
	sb := components.NewStatusBar(80)
	stale := sb.StartDownload("first")
	sb.StartDownload("second") // bumps serial → supersedes
	sb.ClearDownload(stale)    // stale → ignored
	assert.Contains(t, strip(sb.View()), "second")
}

func TestStatusBar_DownloadBeatsStatus(t *testing.T) {
	sb := components.NewStatusBar(80)
	sb.SetStatus("idle status")
	sb.StartDownload("downloading video…")
	view := strip(sb.View())
	assert.Contains(t, view, "downloading video…")
	assert.NotContains(t, view, "idle status")
}
