package store

import (
	"database/sql"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// An address is how to reach a chat the account holds no dialog for: a person
// found by search, or one whose profile was opened from a group. Telegram hands
// it out in answers to those calls and nowhere else, so it is kept the moment
// it arrives. It is kept on disk rather than in memory because a send queued to
// such a person has to survive a restart like any other queued send (#278).
//
// Nothing removes an address once the chat gets a dialog. The chat's own row
// wins from then on, and an address that is never consulted costs one row.

// Address returns the address remembered for a chat, and whether there is one.
func (s *SQLiteStore) Address(chatID int64) (domain.Peer, bool) {
	var peerType, hash int64
	err := s.db.QueryRow(
		`SELECT peer_type, access_hash FROM addresses WHERE chat_id = ?`, chatID,
	).Scan(&peerType, &hash)
	if err == sql.ErrNoRows {
		return domain.Peer{}, false
	}
	if err != nil {
		s.log.Error("read address failed", zap.Int64("chat_id", chatID), zap.Error(err))
		return domain.Peer{}, false
	}
	return domain.Peer{ID: chatID, Type: domain.PeerType(peerType), AccessHash: hash}, true
}

// RememberAddress keeps an address, replacing any earlier one for the same
// chat: the hash Telegram handed out last is the one known to work. A peer
// without a hash names nobody Telegram can reach and is not kept.
func (s *SQLiteStore) RememberAddress(p domain.Peer) {
	if p.ID == 0 || p.AccessHash == 0 {
		return
	}
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO addresses (chat_id, peer_type, access_hash) VALUES (?, ?, ?)`,
		p.ID, int64(p.Type), p.AccessHash,
	)
	if err != nil {
		s.log.Error("remember address failed", zap.Int64("chat_id", p.ID), zap.Error(err))
	}
}
