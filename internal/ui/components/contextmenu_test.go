package components_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultKM() keys.KeyMap { return keys.DefaultKeyMap() }

func keyMsg(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func pressJ() tea.KeyPressMsg     { return keyMsg('j') }
func pressK() tea.KeyPressMsg     { return keyMsg('k') }
func pressR() tea.KeyPressMsg     { return keyMsg('r') }
func pressD() tea.KeyPressMsg     { return keyMsg('d') }
func pressA() tea.KeyPressMsg     { return keyMsg('a') }
func pressM() tea.KeyPressMsg     { return keyMsg('m') }
func pressO() tea.KeyPressMsg     { return keyMsg('o') }
func pressDown() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyDown} }
func pressUp() tea.KeyPressMsg    { return tea.KeyPressMsg{Code: tea.KeyUp} }
func pressEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func pressEsc() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyEsc} }
func pressE() tea.KeyPressMsg     { return keyMsg('e') }
func pressSpace() tea.KeyPressMsg { return keyMsg(' ') }

// --- item display ---

func TestContextMenu_HasForwardItem(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	assert.Contains(t, strip(cm.View()), "forward")
}

func TestContextMenu_F_EmitsForwardRequest(t *testing.T) {
	cm := components.NewContextMenu(7, false, 0, 0, false, false, nil, defaultKM())
	_, cmd := cm.Update(keyMsg('f'))
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ForwardMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 7, req.MsgID)
}

func TestNewContextMenu_IncomingItems(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	view := strip(cm.View())
	assert.Contains(t, view, "reply")
	assert.Contains(t, view, "React")
	assert.Contains(t, view, "delete")
	assert.NotContains(t, view, "edit")
}

func TestNewContextMenu_OutgoingItems(t *testing.T) {
	cm := components.NewContextMenu(1, true, 0, 0, false, false, nil, defaultKM())
	view := strip(cm.View())
	assert.Contains(t, view, "reply")
	assert.Contains(t, view, "React")
	assert.Contains(t, view, "edit")
	assert.Contains(t, view, "delete")
}

func TestNewContextMenu_AccentsHotkeyLetters(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	raw := cm.View()
	// Labels render plain text; the hotkey letter is accent-colored in place,
	// matching the status-bar hint style (btop rules).
	assert.Contains(t, strip(raw), "reply")
	assert.Contains(t, raw, "\x1b[38;5;39", "hotkey letters must be accent-colored")
}

func TestNewContextMenu_ShowsNavHintInBottomBorder(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	view := strip(cm.View())
	// status-bar hint style: "j/k move · select ↵ · esc close"
	assert.Contains(t, view, "j/k")
	assert.Contains(t, view, "select")
	assert.Contains(t, view, "esc")
}

// --- cursor navigation ---

func TestNewContextMenu_CursorStartsAtZero(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	assert.Equal(t, 0, cm.Cursor())
}

func TestContextMenu_J_MovesCursorDown(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	assert.Equal(t, 1, cm.Cursor())
}

func TestContextMenu_DownArrow_MovesCursorDown(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	cm, _ = cm.Update(pressDown())
	require.NotNil(t, cm)
	assert.Equal(t, 1, cm.Cursor())
}

func TestContextMenu_K_MovesCursorUp(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressK())
	require.NotNil(t, cm)
	assert.Equal(t, 0, cm.Cursor())
}

func TestContextMenu_UpArrow_MovesCursorUp(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	cm, _ = cm.Update(pressDown())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressUp())
	require.NotNil(t, cm)
	assert.Equal(t, 0, cm.Cursor())
}

func TestContextMenu_WrapAround_K_FromFirst_GoesToLast(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	// incoming: Reply(0), React(1), Forward(2), Delete(3)
	cm, _ = cm.Update(pressK())
	require.NotNil(t, cm)
	assert.Equal(t, 3, cm.Cursor()) // wrapped to Delete
}

// --- close actions ---

func TestContextMenu_EscFromMain_Closes(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, 0, false, false, nil, defaultKM())
	newCM, cmd := cm.Update(pressEsc())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	assert.IsType(t, components.CloseContextMenuMsg{}, cmd())
}

func TestContextMenu_Space_Closes(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, 0, false, false, nil, defaultKM())
	newCM, cmd := cm.Update(pressSpace())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	assert.IsType(t, components.CloseContextMenuMsg{}, cmd())
}

// --- enter on items ---

func TestContextMenu_Reply_EmitsReplyMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, 0, false, false, nil, defaultKM())
	// cursor starts at 0 (Reply item)
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ReplyMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func TestContextMenu_React_EmitsReactMsgRequestViaEnter(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, 0, false, false, nil, defaultKM())
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ReactMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func TestContextMenu_Edit_EmitsEditMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(42, true, 0, 0, false, false, nil, defaultKM())
	cm, _ = cm.Update(pressJ()) // React
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ()) // Forward
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ()) // Edit
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.EditMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func TestContextMenu_DirectKey_E_EmitsEditMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(42, true, 0, 0, false, false, nil, defaultKM())
	newCM, cmd := cm.Update(pressE())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.EditMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

// --- direct key dispatch ---

func TestContextMenu_DirectKey_R_EmitsReplyMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, 0, false, false, nil, defaultKM())
	cm, _ = cm.Update(pressJ()) // move cursor away
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressR())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ReplyMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func TestContextMenu_Reply_OutgoingMessage_EmitsReplyMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(99, true, 0, 0, false, false, nil, defaultKM())
	// outgoing: Reply(0) React(1) Forward(2) Edit(3) Delete(4); cursor at 0
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ReplyMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 99, req.MsgID)
}

func TestContextMenu_DirectKey_D_IncomingShowsSubMenu(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, 0, false, false, nil, defaultKM())
	newCM, cmd := cm.Update(pressD())
	require.NotNil(t, newCM, "incoming delete opens sub-menu")
	assert.Nil(t, cmd)
	assert.Contains(t, strip(newCM.View()), "For everyone")
	assert.Contains(t, strip(newCM.View()), "For me")
}

func TestContextMenu_DirectKey_D_OutgoingShowsSubMenu(t *testing.T) {
	cm := components.NewContextMenu(42, true, 0, 0, false, false, nil, defaultKM())
	newCM, cmd := cm.Update(pressD())
	require.NotNil(t, newCM, "outgoing delete opens sub-menu")
	assert.Nil(t, cmd)
	assert.Contains(t, strip(newCM.View()), "For everyone")
}

// --- delete (enter navigation) ---

func TestContextMenu_DeleteIncoming_ShowsSubMenu(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, 0, false, false, nil, defaultKM())
	// incoming: Reply(0), React(1), Forward(2), Delete(3)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressEnter())
	require.NotNil(t, newCM, "Delete on incoming opens sub-menu")
	assert.Nil(t, cmd)
	assert.Contains(t, strip(newCM.View()), "For everyone")
	assert.Contains(t, strip(newCM.View()), "For me")
}

func TestContextMenu_DeleteOutgoing_ShowsSubPrompt(t *testing.T) {
	cm := components.NewContextMenu(42, true, 0, 0, false, false, nil, defaultKM())
	// outgoing: Reply(0), React(1), Forward(2), Edit(3), Delete(4)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressEnter())
	require.NotNil(t, newCM)
	assert.Nil(t, cmd)
	view := strip(newCM.View())
	assert.Contains(t, view, "For everyone")
	assert.Contains(t, view, "For me")
	assert.NotContains(t, view, "reply")
}

// --- delete sub-menu ---

func TestContextMenu_DeleteSub_ShowsItemKeys(t *testing.T) {
	cm := navigateToDeleteSubPrompt(t)
	view := strip(cm.View())
	// "a" is not a letter of "For everyone", so the hotkey renders as an accented
	// prefix (btop rule). "For me" contains "m", highlighted in place.
	assert.Contains(t, view, "a For everyone")
	assert.Contains(t, view, "For me")
}

func TestContextMenu_DeleteSub_ForEveryone_EmitsDeleteRevoke(t *testing.T) {
	cm := navigateToDeleteSubPrompt(t)
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.DeleteMsgRequest)
	require.True(t, ok)
	assert.True(t, req.Revoke)
}

func TestContextMenu_DeleteSub_ForMe_EmitsDelete(t *testing.T) {
	cm := navigateToDeleteSubPrompt(t)
	cm, _ = cm.Update(pressJ()) // For me
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.DeleteMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 99, req.MsgID)
	assert.False(t, req.Revoke)
}

func TestContextMenu_DeleteSub_DirectKey_A_ForEveryone(t *testing.T) {
	cm := navigateToDeleteSubPrompt(t)
	newCM, cmd := cm.Update(pressA())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.DeleteMsgRequest)
	require.True(t, ok)
	assert.True(t, req.Revoke)
}

func TestContextMenu_DeleteSub_DirectKey_M_ForMe(t *testing.T) {
	cm := navigateToDeleteSubPrompt(t)
	newCM, cmd := cm.Update(pressM())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.DeleteMsgRequest)
	require.True(t, ok)
	assert.False(t, req.Revoke)
}

func TestContextMenu_DeleteSub_SeparatorSkipped_Down(t *testing.T) {
	cm := navigateToDeleteSubPrompt(t)
	cm, _ = cm.Update(pressJ()) // For me (1)
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ()) // skip sep(2), land on Cancel(3)
	require.NotNil(t, cm)
	assert.Equal(t, 3, cm.Cursor())
}

func TestContextMenu_DeleteSub_SeparatorSkipped_Up(t *testing.T) {
	cm := navigateToDeleteSubPrompt(t)
	cm, _ = cm.Update(pressJ()) // For me
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ()) // Cancel (skipping sep)
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressK()) // skip sep going up, land on For me
	require.NotNil(t, cm)
	assert.Equal(t, 1, cm.Cursor())
}

func TestContextMenu_EscFromSubPrompt_ReturnsToMain(t *testing.T) {
	cm := navigateToDeleteSubPrompt(t)
	newCM, cmd := cm.Update(pressEsc())
	require.NotNil(t, newCM)
	assert.Nil(t, cmd)
	view := strip(newCM.View())
	assert.Contains(t, view, "reply")
	assert.NotContains(t, view, "For me")
}

func TestContextMenu_View_ReturnsNonEmpty(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	assert.NotEmpty(t, cm.View())
}

func pressG() tea.KeyPressMsg { return keyMsg('g') }
func pressT() tea.KeyPressMsg { return keyMsg('t') }

func TestNewContextMenu_IsReply_ShowsJumpToOriginal(t *testing.T) {
	cm := components.NewContextMenu(1, false, 42, 0, false, false, nil, defaultKM())
	view := strip(cm.View())
	assert.Contains(t, view, "Jump to original")
}

func TestNewContextMenu_NotReply_NoJumpToOriginal(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	view := strip(cm.View())
	assert.NotContains(t, view, "Jump to original")
}

func TestContextMenu_React_EmitsReactMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, 0, false, false, nil, defaultKM())
	// items: Reply, React — cursor on React after one J
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ReactMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func TestContextMenu_DirectKey_T_EmitsReactMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, 0, false, false, nil, defaultKM())
	newCM, cmd := cm.Update(pressT())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ReactMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func TestContextMenu_JumpToOriginal_EmitsJumpToMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(1, false, 42, 0, false, false, nil, defaultKM())
	// Jump to original is item 0 (prepended), cursor starts at 0.
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.JumpToMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func TestContextMenu_DirectKey_G_JumpsToOriginal(t *testing.T) {
	cm := components.NewContextMenu(1, false, 42, 0, false, false, nil, defaultKM())
	newCM, cmd := cm.Update(pressG())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.JumpToMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func navigateToDeleteSubPrompt(t *testing.T) *components.ContextMenu {
	t.Helper()
	cm := components.NewContextMenu(99, true, 0, 0, false, false, nil, defaultKM())
	// outgoing: Reply(0) React(1) Forward(2) Edit(3) Delete(4)
	cm, _ = cm.Update(pressJ()) // React
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ()) // Forward
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ()) // Edit
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ()) // Delete
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressEnter()) // open sub-menu
	require.NotNil(t, cm)
	return cm
}

func navigateToIncomingDeleteSubPrompt(t *testing.T) *components.ContextMenu {
	t.Helper()
	cm := components.NewContextMenu(77, false, 0, 0, false, false, nil, defaultKM())
	// incoming: Reply(0), React(1), Forward(2), Delete(3)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressEnter()) // open sub-menu
	require.NotNil(t, cm)
	return cm
}

func TestContextMenu_IncomingDeleteSub_ForEveryone_EmitsRevoke(t *testing.T) {
	cm := navigateToIncomingDeleteSubPrompt(t)
	newCM, cmd := cm.Update(pressA())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.DeleteMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 77, req.MsgID)
	assert.True(t, req.Revoke)
}

func TestContextMenu_IncomingDeleteSub_ForMe_EmitsNoRevoke(t *testing.T) {
	cm := navigateToIncomingDeleteSubPrompt(t)
	newCM, cmd := cm.Update(pressM())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.DeleteMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 77, req.MsgID)
	assert.False(t, req.Revoke)
}

func TestContextMenu_IncomingDeleteSub_EscReturnsToMain(t *testing.T) {
	cm := navigateToIncomingDeleteSubPrompt(t)
	newCM, cmd := cm.Update(pressEsc())
	require.NotNil(t, newCM)
	assert.Nil(t, cmd)
	assert.Contains(t, strip(newCM.View()), "reply")
	assert.NotContains(t, strip(newCM.View()), "For me")
}

// --- media actions per kind ---

func photoTargets() []components.OpenTarget {
	return []components.OpenTarget{{Kind: components.OpenTargetPhoto, Label: "Photo"}}
}

func videoTargets() []components.OpenTarget {
	return []components.OpenTarget{{Kind: components.OpenTargetVideo, Label: "Video"}}
}

func TestNewContextMenu_PhotoMessage_ShowsAllThreeActions(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, store.MediaPhoto, true, false, photoTargets(), defaultKM())
	view := strip(cm.View())
	assert.Contains(t, view, "open photo")
	assert.Contains(t, view, "Open photo externally")
	assert.Contains(t, view, "save photo (download)")
}

func TestNewContextMenu_NoMedia_HidesMediaActions(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	view := strip(cm.View())
	assert.NotContains(t, view, "externally")
	assert.NotContains(t, view, "Download")
}

func TestNewContextMenu_VideoMessage_ShowsAllThreeActions(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, store.MediaVideo, true, false, videoTargets(), defaultKM())
	view := strip(cm.View())
	assert.Contains(t, view, "open video")
	assert.Contains(t, view, "Open video externally")
	assert.Contains(t, view, "save video (download)")
}

func TestContextMenu_Photo_OpenExternal_EmitsOpenExternalRequest(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, store.MediaPhoto, true, false, nil, defaultKM())
	newCM, cmd := cm.Update(keyMsg('O'))
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	_, ok := cmd().(components.OpenExternalRequest)
	require.True(t, ok, "shift-O must emit OpenExternalRequest")
}

func TestContextMenu_Video_OpenInApp_EmitsOpenInViewerRequest(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, store.MediaVideo, true, false, videoTargets(), defaultKM())
	newCM, cmd := cm.Update(pressO())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	_, ok := cmd().(components.OpenInViewerRequest)
	require.True(t, ok, "o must emit OpenInViewerRequest (in-app modal)")
}

func TestContextMenu_Photo_OpenInApp_EmitsOpenInViewerRequest(t *testing.T) {
	cm := components.NewContextMenu(7, false, 0, store.MediaPhoto, true, false, photoTargets(), defaultKM())
	newCM, cmd := cm.Update(pressO())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	_, ok := cmd().(components.OpenInViewerRequest)
	require.True(t, ok, "o must emit OpenInViewerRequest for a photo (in-app modal)")
}

func TestNewContextMenu_VoiceMessage_ShowsPlayAndDownload(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, store.MediaVoice, true, false, nil, defaultKM())
	view := strip(cm.View())
	assert.Contains(t, view, "play voice")
	assert.Contains(t, view, "save voice (download)")
}

func TestNewContextMenu_GIFMessage_ShowsDownloadOnly(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, store.MediaGIF, true, false, nil, defaultKM())
	view := strip(cm.View())
	assert.Contains(t, view, "save GIF (download)")
	assert.NotContains(t, view, "externally")
}

func TestNewContextMenu_StickerMessage_ShowsNoMediaActions(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, store.MediaSticker, true, false, nil, defaultKM())
	view := strip(cm.View())
	assert.NotContains(t, view, "Download")
	assert.NotContains(t, view, "Open externally")
}

func TestNewContextMenu_HasText_ShowsCopy(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, true, nil, defaultKM())
	assert.Contains(t, strip(cm.View()), "Copy")
}

func TestNewContextMenu_NoText_HidesCopy(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, false, nil, defaultKM())
	assert.NotContains(t, strip(cm.View()), "Copy")
}

func TestContextMenu_Copy_EmitsCopyMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, 0, false, true, nil, defaultKM())
	// 'y' is the Copy binding in the context menu.
	newCM, cmd := cm.Update(keyMsg('y'))
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	_, ok := cmd().(components.CopyMsgRequest)
	assert.True(t, ok, "selecting Copy must emit CopyMsgRequest")
}

func linkTarget() []components.OpenTarget {
	return []components.OpenTarget{{Kind: components.OpenTargetLink, Label: "https://x.com", URL: "https://x.com"}}
}

func TestNewContextMenu_SingleLink_ShowsOpenLink(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, true, linkTarget(), defaultKM())
	assert.Contains(t, strip(cm.View()), "open link")
}

func TestNewContextMenu_NoOpenTargets_HidesOpen(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, 0, false, true, nil, defaultKM())
	assert.NotContains(t, strip(cm.View()), "Open")
}

func TestContextMenu_Link_Open_EmitsOpenInViewerRequest(t *testing.T) {
	cm := components.NewContextMenu(5, false, 0, 0, false, true, linkTarget(), defaultKM())
	newCM, cmd := cm.Update(pressO())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	_, ok := cmd().(components.OpenInViewerRequest)
	assert.True(t, ok, "o on a link message must emit OpenInViewerRequest (routed to handleOpen)")
}

func TestNewContextMenu_FileShowsDownload(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, store.MediaFile, true, false, nil, defaultKM())
	assert.Contains(t, strip(cm.View()), "save file (download)")
}

func TestContextMenu_Download_EmitsDownloadFileRequest(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, store.MediaFile, true, false, nil, defaultKM())
	// 's' is the Download binding in the context menu.
	_, cmd := cm.Update(keyMsg('s'))
	require.NotNil(t, cmd)
	_, ok := cmd().(components.DownloadFileRequest)
	assert.True(t, ok, "selecting Download must emit DownloadFileRequest")
}
