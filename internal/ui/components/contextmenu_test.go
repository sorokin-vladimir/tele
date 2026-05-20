package components_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
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
func pressDown() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyDown} }
func pressUp() tea.KeyPressMsg    { return tea.KeyPressMsg{Code: tea.KeyUp} }
func pressEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func pressEsc() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyEsc} }
func pressE() tea.KeyPressMsg     { return keyMsg('e') }
func pressSpace() tea.KeyPressMsg { return keyMsg(' ') }

// --- item display ---

func TestNewContextMenu_IncomingItems(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	view := cm.View()
	assert.Contains(t, view, "Reply")
	assert.Contains(t, view, "React")
	assert.Contains(t, view, "Delete")
	assert.NotContains(t, view, "Edit")
}

func TestNewContextMenu_OutgoingItems(t *testing.T) {
	cm := components.NewContextMenu(1, true, 0, defaultKM())
	view := cm.View()
	assert.Contains(t, view, "Reply")
	assert.Contains(t, view, "React")
	assert.Contains(t, view, "Edit")
	assert.Contains(t, view, "Delete")
}

func TestNewContextMenu_ShowsKeyBindings(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	view := cm.View()
	assert.Contains(t, view, "r -> Reply")
	assert.Contains(t, view, "t -> React")
	assert.Contains(t, view, "d -> Delete")
}

func TestNewContextMenu_ShowsNavHintInBottomBorder(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	view := cm.View()
	assert.Contains(t, view, "j/k")
	assert.Contains(t, view, "enter")
	assert.Contains(t, view, "esc")
}

// --- cursor navigation ---

func TestNewContextMenu_CursorStartsAtZero(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	assert.Equal(t, 0, cm.Cursor())
}

func TestContextMenu_J_MovesCursorDown(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	assert.Equal(t, 1, cm.Cursor())
}

func TestContextMenu_DownArrow_MovesCursorDown(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	cm, _ = cm.Update(pressDown())
	require.NotNil(t, cm)
	assert.Equal(t, 1, cm.Cursor())
}

func TestContextMenu_K_MovesCursorUp(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressK())
	require.NotNil(t, cm)
	assert.Equal(t, 0, cm.Cursor())
}

func TestContextMenu_UpArrow_MovesCursorUp(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	cm, _ = cm.Update(pressDown())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressUp())
	require.NotNil(t, cm)
	assert.Equal(t, 0, cm.Cursor())
}

func TestContextMenu_WrapAround_K_FromFirst_GoesToLast(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	// incoming: Reply(0), React(1), Delete(2)
	cm, _ = cm.Update(pressK())
	require.NotNil(t, cm)
	assert.Equal(t, 2, cm.Cursor()) // wrapped to Delete
}

// --- close actions ---

func TestContextMenu_EscFromMain_Closes(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, defaultKM())
	newCM, cmd := cm.Update(pressEsc())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	assert.IsType(t, components.CloseContextMenuMsg{}, cmd())
}

func TestContextMenu_Space_Closes(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, defaultKM())
	newCM, cmd := cm.Update(pressSpace())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	assert.IsType(t, components.CloseContextMenuMsg{}, cmd())
}

// --- enter on items ---

func TestContextMenu_Reply_EmitsReplyMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, defaultKM())
	// cursor starts at 0 (Reply item)
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ReplyMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func TestContextMenu_React_EmitsReactMsgRequestViaEnter(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, defaultKM())
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
	cm := components.NewContextMenu(42, true, 0, defaultKM())
	cm, _ = cm.Update(pressJ()) // React
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
	cm := components.NewContextMenu(42, true, 0, defaultKM())
	newCM, cmd := cm.Update(pressE())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.EditMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

// --- direct key dispatch ---

func TestContextMenu_DirectKey_R_EmitsReplyMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, defaultKM())
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
	cm := components.NewContextMenu(99, true, 0, defaultKM())
	// outgoing: Reply(0) React(1) Edit(2) Delete(3); cursor at 0
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ReplyMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 99, req.MsgID)
}

func TestContextMenu_DirectKey_D_IncomingShowsSubMenu(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, defaultKM())
	newCM, cmd := cm.Update(pressD())
	require.NotNil(t, newCM, "incoming delete opens sub-menu")
	assert.Nil(t, cmd)
	assert.Contains(t, newCM.View(), "For everyone")
	assert.Contains(t, newCM.View(), "For me")
}

func TestContextMenu_DirectKey_D_OutgoingShowsSubMenu(t *testing.T) {
	cm := components.NewContextMenu(42, true, 0, defaultKM())
	newCM, cmd := cm.Update(pressD())
	require.NotNil(t, newCM, "outgoing delete opens sub-menu")
	assert.Nil(t, cmd)
	assert.Contains(t, newCM.View(), "For everyone")
}

// --- delete (enter navigation) ---

func TestContextMenu_DeleteIncoming_ShowsSubMenu(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, defaultKM())
	// incoming: Reply(0), React(1), Delete(2)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressEnter())
	require.NotNil(t, newCM, "Delete on incoming opens sub-menu")
	assert.Nil(t, cmd)
	assert.Contains(t, newCM.View(), "For everyone")
	assert.Contains(t, newCM.View(), "For me")
}

func TestContextMenu_DeleteOutgoing_ShowsSubPrompt(t *testing.T) {
	cm := components.NewContextMenu(42, true, 0, defaultKM())
	// outgoing: Reply(0), React(1), Edit(2), Delete(3)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressEnter())
	require.NotNil(t, newCM)
	assert.Nil(t, cmd)
	view := newCM.View()
	assert.Contains(t, view, "For everyone")
	assert.Contains(t, view, "For me")
	assert.NotContains(t, view, "Reply")
}

// --- delete sub-menu ---

func TestContextMenu_DeleteSub_ShowsItemKeys(t *testing.T) {
	cm := navigateToDeleteSubPrompt(t)
	view := cm.View()
	assert.Contains(t, view, "a -> For everyone")
	assert.Contains(t, view, "m -> For me")
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
	view := newCM.View()
	assert.Contains(t, view, "Reply")
	assert.NotContains(t, view, "For me")
}

func TestContextMenu_View_ReturnsNonEmpty(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	assert.NotEmpty(t, cm.View())
}

func pressG() tea.KeyPressMsg { return keyMsg('g') }
func pressT() tea.KeyPressMsg { return keyMsg('t') }

func TestNewContextMenu_IsReply_ShowsJumpToOriginal(t *testing.T) {
	cm := components.NewContextMenu(1, false, 42, defaultKM())
	view := cm.View()
	assert.Contains(t, view, "Jump to original")
}

func TestNewContextMenu_NotReply_NoJumpToOriginal(t *testing.T) {
	cm := components.NewContextMenu(1, false, 0, defaultKM())
	view := cm.View()
	assert.NotContains(t, view, "Jump to original")
}

func TestContextMenu_React_EmitsReactMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(42, false, 0, defaultKM())
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
	cm := components.NewContextMenu(42, false, 0, defaultKM())
	newCM, cmd := cm.Update(pressT())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ReactMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func TestContextMenu_JumpToOriginal_EmitsJumpToMsgRequest(t *testing.T) {
	cm := components.NewContextMenu(1, false, 42, defaultKM())
	// Jump to original is item 0 (prepended), cursor starts at 0.
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.JumpToMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func TestContextMenu_DirectKey_G_JumpsToOriginal(t *testing.T) {
	cm := components.NewContextMenu(1, false, 42, defaultKM())
	newCM, cmd := cm.Update(pressG())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.JumpToMsgRequest)
	require.True(t, ok)
	assert.Equal(t, 42, req.MsgID)
}

func navigateToDeleteSubPrompt(t *testing.T) *components.ContextMenu {
	t.Helper()
	cm := components.NewContextMenu(99, true, 0, defaultKM())
	// outgoing: Reply(0) React(1) Edit(2) Delete(3)
	cm, _ = cm.Update(pressJ()) // React
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
	cm := components.NewContextMenu(77, false, 0, defaultKM())
	// incoming: Reply(0), React(1), Delete(2)
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
	assert.Contains(t, newCM.View(), "Reply")
	assert.NotContains(t, newCM.View(), "For me")
}
