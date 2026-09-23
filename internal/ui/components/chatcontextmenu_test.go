package components_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

func TestChatMenu_ReadLabelForUnreadChat(t *testing.T) {
	chat := project.ChatRow{ID: 1, Unread: 3, IsUser: true}
	cm := components.NewChatContextMenu(chat, nil, keys.DefaultKeyMap())
	assert.Contains(t, cm.View(), "Mark as read")
}

func TestChatMenu_UnreadLabelForReadChat(t *testing.T) {
	chat := project.ChatRow{ID: 1, IsUser: true}
	cm := components.NewChatContextMenu(chat, nil, keys.DefaultKeyMap())
	assert.Contains(t, cm.View(), "Mark as unread")
}

func TestChatMenu_MuteToggleLabel(t *testing.T) {
	muted := components.NewChatContextMenu(project.ChatRow{ID: 1, Muted: true}, nil, keys.DefaultKeyMap())
	assert.Contains(t, muted.View(), "Unmute")

	unmuted := components.NewChatContextMenu(project.ChatRow{ID: 1}, nil, keys.DefaultKeyMap())
	v := unmuted.View()
	assert.Contains(t, v, "Mute")
	assert.NotContains(t, v, "Unmute")
}

func TestChatMenu_EmitsToggleMuteRequest(t *testing.T) {
	chat := project.ChatRow{ID: 1, IsUser: true}
	cm := components.NewChatContextMenu(chat, nil, keys.DefaultKeyMap())
	_, cmd := cm.Update(keyMsg('m')) // direct key -> Mute
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ToggleMuteRequest)
	require.True(t, ok)
	assert.True(t, req.Muted)
	assert.Equal(t, int64(1), req.ChatID)
}

func TestChatMenu_FolderSubmenuToggle(t *testing.T) {
	folders := []domain.FolderFilter{{ID: 7, Title: "Work", IncludePeers: []int64{1}}}
	chat := project.ChatRow{ID: 1, IsUser: true}
	cm := components.NewChatContextMenu(chat, folders, keys.DefaultKeyMap())

	// open the folder submenu via direct key 'f'
	_, cmd := cm.Update(keyMsg('f'))
	require.Nil(t, cmd) // opening a submenu does not emit
	assert.Contains(t, cm.View(), "✓ Work")

	// cursor starts at the first folder; Enter toggles membership (remove).
	_, cmd = cm.Update(pressEnter())
	require.NotNil(t, cmd)
	req, ok := cmd().(components.AddToFolderRequest)
	require.True(t, ok)
	assert.Equal(t, 7, req.FilterID)
	assert.False(t, req.Add)
	assert.Equal(t, int64(1), req.ChatID)
}

// The menu opens before the owner has answered with the folders. "Add to
// folder" appears once they arrive, and the cursor stays on the item it was on.
func TestChatMenu_FoldersArriveAfterTheMenuOpens(t *testing.T) {
	cm := components.NewChatContextMenu(project.ChatRow{ID: 1, IsUser: true}, nil, keys.DefaultKeyMap())
	require.NotContains(t, cm.View(), "Add to folder")
	cm.Update(pressDown()) // Mark as unread -> Mute
	before := cm.Cursor()

	cm.SetFolders([]domain.FolderFilter{{ID: 7, Title: "Work"}})

	assert.Contains(t, cm.View(), "Add to folder")
	assert.Equal(t, before, cm.Cursor(), "Mute sits above the new item and keeps the cursor")
	_, cmd := cm.Update(pressEnter())
	require.NotNil(t, cmd)
	_, ok := cmd().(components.ToggleMuteRequest)
	assert.True(t, ok)
}

func TestChatMenu_ArchiveEntry(t *testing.T) {
	cm := components.NewChatContextMenu(project.ChatRow{ID: 1}, nil, keys.DefaultKeyMap())
	assert.Contains(t, cm.View(), "Archive")

	cmA := components.NewChatContextMenu(project.ChatRow{ID: 1, Archived: true}, nil, keys.DefaultKeyMap())
	assert.Contains(t, cmA.View(), "Unarchive")
}

func TestChatMenu_EmitsToggleArchiveRequest(t *testing.T) {
	cm := components.NewChatContextMenu(project.ChatRow{ID: 1, IsUser: true}, nil, keys.DefaultKeyMap())
	_, cmd := cm.Update(keyMsg('a')) // direct key -> Archive
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ToggleArchiveRequest)
	require.True(t, ok)
	assert.True(t, req.Archived)
	assert.Equal(t, int64(1), req.ChatID)
}
