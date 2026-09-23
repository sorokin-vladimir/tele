package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// handleChatMenuRequest applies an optimistic store mutation for a
// chat-menu action, refreshes the visible panes, and returns the async
// Telegram command. The returned bool is false when msg is not a
// chat-menu request.
func (m RootModel) handleChatMenuRequest(msg tea.Msg) (RootModel, tea.Cmd, bool) {
	switch req := msg.(type) {
	case components.ToggleMuteRequest:
		m.chatMenu = nil
		if m.owner == nil {
			return m, nil, true
		}
		ctx, owner, chatID, muted := m.ctx, m.owner, req.ChatID, req.Muted
		return m, func() tea.Msg {
			if err := owner.SetMuted(ctx, chatID, muted); err != nil {
				return errStatus("mute", err)
			}
			return nil
		}, true

	case components.ToggleUnreadRequest:
		m.chatMenu = nil
		if m.owner == nil {
			return m, nil, true
		}
		ctx, owner, chatID := m.ctx, m.owner, req.ChatID
		if req.Unread {
			return m, func() tea.Msg {
				if err := owner.SetUnreadMark(ctx, chatID, true); err != nil {
					return errStatus("mark unread", err)
				}
				return nil
			}, true
		}
		// Mark as read: a maxID of 0 reads the chat outright, clearing both the
		// count and the manual mark.
		return m, func() tea.Msg {
			if err := owner.MarkRead(ctx, chatID, 0); err != nil {
				return errStatus("mark read", err)
			}
			return nil
		}, true

	case components.AddToFolderRequest:
		m.chatMenu = nil
		if m.owner == nil {
			return m, nil, true
		}
		ctx, owner := m.ctx, m.owner
		filterID, chatID, add := req.FilterID, req.ChatID, req.Add
		return m, func() tea.Msg {
			if err := owner.AddToFolder(ctx, filterID, chatID, add); err != nil {
				return errStatus("folder", err)
			}
			return nil
		}, true

	case components.ToggleArchiveRequest:
		m.chatMenu = nil
		if m.owner == nil {
			return m, nil, true
		}
		ctx, owner, chatID, archived := m.ctx, m.owner, req.ChatID, req.Archived
		return m, func() tea.Msg {
			if err := owner.SetArchived(ctx, chatID, archived); err != nil {
				return errStatus("archive", err)
			}
			return nil
		}, true
	}
	return m, nil, false
}

// refreshChatPanes is gone: an optimistic store write commits, the commit
// rebuilds the subscribed projections, and the resulting delta repaints the
// panes. Nothing has to remember to refresh.
//
// toggleInt64 is gone too: folder membership is state, so it moved to
// core/state as toggleChatID (#198).
