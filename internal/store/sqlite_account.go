package store

import (
	"fmt"

	"go.uber.org/zap"
)

// SetOwnerID records the signed-in user for UI and diagnostic consumers. Any
// previous account's files are removed before this store is opened.
func (s *SQLiteStore) SetOwnerID(ownerID int64) {
	_, err := s.db.Exec(`
		INSERT INTO metadata (key, value) VALUES ('owner_id', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, fmt.Sprint(ownerID))
	if err != nil {
		s.log.Error("record owner_id failed", zap.Error(err))
	}
}
