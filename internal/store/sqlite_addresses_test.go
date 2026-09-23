package store_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// A send queued to someone with no dialog must still be addressable after a
// restart, so the address outlives the process that learned it (#278).
func TestSQLite_Address_SurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	log := zap.NewNop()

	s, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	s.RememberAddress(domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 4242})
	_ = s.Close()

	s2, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	got, ok := s2.Address(42)
	require.True(t, ok)
	assert.Equal(t, domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 4242}, got)
}

func TestSQLite_Address_UnknownChat(t *testing.T) {
	s := newTestSQLite(t)

	_, ok := s.Address(42)

	assert.False(t, ok)
}

// Telegram may hand out a new hash for the same person; the latest one is the
// one that works.
func TestSQLite_Address_LatestWins(t *testing.T) {
	s := newTestSQLite(t)

	s.RememberAddress(domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 1})
	s.RememberAddress(domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 2})

	got, ok := s.Address(42)
	require.True(t, ok)
	assert.Equal(t, int64(2), got.AccessHash)
}

// An address without a hash names nobody Telegram can reach, and keeping it
// would only make a later lookup succeed with something useless.
func TestSQLite_Address_WithoutAHashIsNotKept(t *testing.T) {
	s := newTestSQLite(t)

	s.RememberAddress(domain.Peer{ID: 42, Type: domain.PeerUser})

	_, ok := s.Address(42)
	assert.False(t, ok)
}
