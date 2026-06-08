package store

import "go.uber.org/zap"

// NewMemory returns an in-memory Store backed by an ephemeral SQLite database.
// It exists for tests and other throwaway contexts; production uses NewSQLite
// with a file path. Keeping a single Store implementation avoids behavioural
// drift between a hand-rolled memory store and the real persistence path.
func NewMemory() Store {
	s, err := NewSQLite(":memory:", zap.NewNop())
	if err != nil {
		// Opening an in-memory SQLite database has no filesystem dependency and
		// cannot fail in practice; treat any failure as a programmer error.
		panic(err)
	}
	return s
}
