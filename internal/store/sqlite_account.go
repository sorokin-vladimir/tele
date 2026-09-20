package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"go.uber.org/zap"
)

var clearAccountStatements = []string{
	`DELETE FROM messages`,
	`DELETE FROM chats`,
	`DELETE FROM folder_filters`,
	`DELETE FROM chat_gap`,
	`DELETE FROM update_state`,
	`DELETE FROM channel_pts`,
	`DELETE FROM channel_access_hash`,
}

const (
	accountResetVersionKey = "account_reset_version"
	accountResetVersion    = "1"
)

// ClearForNewAccount clears all account-specific data when ownerID differs from the stored one.
// If no owner is recorded yet (first launch with this version), it just records ownerID.
func (s *SQLiteStore) ClearForNewAccount(ownerID int64) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin account reset: %w", err)
	}
	rollback := func(err error) error {
		_ = tx.Rollback()
		return err
	}

	var raw string
	err = tx.QueryRow(`SELECT value FROM metadata WHERE key = 'owner_id'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.Exec(
			`INSERT INTO metadata (key, value) VALUES ('owner_id', ?)`, strconv.FormatInt(ownerID, 10),
		); err != nil {
			return rollback(fmt.Errorf("record owner_id: %w", err))
		}
		if _, err := tx.Exec(
			`INSERT INTO metadata (key, value) VALUES (?, ?)`, accountResetVersionKey, accountResetVersion,
		); err != nil {
			return rollback(fmt.Errorf("record account reset version: %w", err))
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit owner_id: %w", err)
		}
		return nil
	}
	if err != nil {
		return rollback(fmt.Errorf("read owner_id: %w", err))
	}
	storedID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return rollback(fmt.Errorf("parse owner_id: %w", err))
	}

	var resetVersion string
	err = tx.QueryRow(`SELECT value FROM metadata WHERE key = ?`, accountResetVersionKey).Scan(&resetVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return rollback(fmt.Errorf("read account reset version: %w", err))
	}
	if storedID == ownerID && resetVersion == accountResetVersion {
		return tx.Commit()
	}

	if storedID == ownerID {
		s.log.Info("upgrading account data isolation", zap.Int64("owner", ownerID))
	} else {
		s.log.Info("account changed, clearing store", zap.Int64("old", storedID), zap.Int64("new", ownerID))
	}
	for _, statement := range clearAccountStatements {
		if _, err := tx.Exec(statement); err != nil {
			return rollback(fmt.Errorf("clear account data: %w", err))
		}
	}
	if _, err := tx.Exec(
		`UPDATE metadata SET value = ? WHERE key = 'owner_id'`, strconv.FormatInt(ownerID, 10),
	); err != nil {
		return rollback(fmt.Errorf("update owner_id: %w", err))
	}
	if _, err := tx.Exec(
		`INSERT INTO metadata (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		accountResetVersionKey,
		accountResetVersion,
	); err != nil {
		return rollback(fmt.Errorf("update account reset version: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit account reset: %w", err)
	}

	s.resetAccountMemory()
	return nil
}

// resetAccountMemory drops every cache and pending write owned by the previous account.
func (s *SQLiteStore) resetAccountMemory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chats = make(map[int64]domain.Chat)
	s.messages = make(map[int64][]domain.Message)
	s.unreadReactionMsgs = make(map[int64]map[int]struct{})
	s.unreadMentionMsgs = make(map[int64]map[int]struct{})
	s.baselineUnread = make(map[int64]int)
	s.unreadMsgs = make(map[int64]map[int]struct{})
	s.sortedIDs = nil
	s.orderDirty = true
	s.msgChat = make(map[int]int64)
	s.msgFloor = make(map[int64]int)
	s.dirtyPersist = make(map[int64]struct{})
	s.loaded = make(map[int64]bool)
	s.dirtyMsgs = make(map[int64]map[int]struct{})
	s.deletedMsgs = make(map[int64]map[int]struct{})
	s.deletingMsgs = make(map[int64]map[int]struct{})
}
