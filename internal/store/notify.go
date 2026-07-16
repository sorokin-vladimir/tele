package store

import "time"

// NotifyFreshnessWindow bounds how old an incoming message may be and still
// raise a notification. Catch-up/backlog messages recovered via getDifference
// after an idle period carry their original (old) send time, so anything older
// than this window is treated as catch-up and stays silent (#123). Live
// delivery latency is typically well under this.
const NotifyFreshnessWindow = 10 * time.Second

// Notifiable applies the gating shared by OS and in-app notifications: the chat
// must exist, not be the open one, not be muted/archived, and the triggering
// event must be fresh (not catch-up backlog recovered after idle).
func Notifiable(st Store, chatID, currentChatID int64, eventTime, now time.Time) bool {
	if chatID == currentChatID {
		return false
	}
	chat, ok := st.GetChat(chatID)
	if !ok {
		return false
	}
	if chat.IsMuted || chat.IsArchived {
		return false
	}
	if eventTime.IsZero() || now.Sub(eventTime) > NotifyFreshnessWindow {
		return false
	}
	return true
}
