package store

import (
	"database/sql"
	"fmt"

	"go.uber.org/zap"
)

// ClearForNewAccount clears all account-specific data when ownerID differs from the stored one.
// If no owner is recorded yet (first launch with this version), it just records ownerID.
func (s *SQLiteStore) ClearForNewAccount(ownerID int64) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = 'owner_id'`).Scan(&raw)
	if err == sql.ErrNoRows {
		_, _ = s.db.Exec(`INSERT INTO metadata (key, value) VALUES ('owner_id', ?)`, fmt.Sprint(ownerID))
		return
	}
	if err != nil {
		s.log.Error("read owner_id failed", zap.Error(err))
		return
	}
	var storedID int64
	_, _ = fmt.Sscan(raw, &storedID)
	if storedID == ownerID {
		return
	}

	s.log.Info("account changed, clearing store", zap.Int64("old", storedID), zap.Int64("new", ownerID))

	s.mu.Lock()
	s.chats = make(map[int64]Chat)
	s.messages = make(map[int64][]Message)
	s.unreadReactionMsgs = make(map[int64]map[int]struct{})
	s.msgChat = make(map[int]int64)
	s.dirtyPersist = make(map[int64]struct{})
	s.sortedIDs = nil
	s.orderDirty = true
	s.mu.Unlock()

	if _, err = s.db.Exec(`DELETE FROM chats`); err != nil {
		s.log.Error("clear chats failed", zap.Error(err))
	}
	if _, err = s.db.Exec(`DELETE FROM folder_filters`); err != nil {
		s.log.Error("clear folder_filters failed", zap.Error(err))
	}
	if _, err = s.db.Exec(`UPDATE metadata SET value = ? WHERE key = 'owner_id'`, fmt.Sprint(ownerID)); err != nil {
		s.log.Error("update owner_id failed", zap.Error(err))
	}
}
