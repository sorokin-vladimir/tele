package store_test

import (
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func freshStoreWithChat(c store.Chat) store.Store {
	s := store.NewMemory()
	s.SetChat(c)
	return s
}

func TestNotifiable_FreshUnmutedOtherChat(t *testing.T) {
	s := freshStoreWithChat(store.Chat{ID: 7, Title: "Alice"})
	now := time.Now()
	if !store.Notifiable(s, 7, 0, now, now) {
		t.Fatal("a fresh message in an unmuted, non-current chat should notify")
	}
}

func TestNotifiable_CurrentChatSuppressed(t *testing.T) {
	s := freshStoreWithChat(store.Chat{ID: 7})
	now := time.Now()
	if store.Notifiable(s, 7, 7, now, now) {
		t.Fatal("current chat must not notify")
	}
}

func TestNotifiable_MissingChatSuppressed(t *testing.T) {
	s := store.NewMemory()
	now := time.Now()
	if store.Notifiable(s, 7, 0, now, now) {
		t.Fatal("missing chat must not notify")
	}
}

func TestNotifiable_MutedSuppressed(t *testing.T) {
	s := freshStoreWithChat(store.Chat{ID: 7, IsMuted: true})
	now := time.Now()
	if store.Notifiable(s, 7, 0, now, now) {
		t.Fatal("muted chat must not notify")
	}
}

func TestNotifiable_ArchivedSuppressed(t *testing.T) {
	s := freshStoreWithChat(store.Chat{ID: 7, IsArchived: true})
	now := time.Now()
	if store.Notifiable(s, 7, 0, now, now) {
		t.Fatal("archived chat must not notify")
	}
}

func TestNotifiable_StaleSuppressed(t *testing.T) {
	s := freshStoreWithChat(store.Chat{ID: 7})
	now := time.Now()
	stale := now.Add(-store.NotifyFreshnessWindow - time.Second)
	if store.Notifiable(s, 7, 0, stale, now) {
		t.Fatal("stale catch-up message must not notify")
	}
}

func TestNotifiable_ZeroTimeSuppressed(t *testing.T) {
	s := freshStoreWithChat(store.Chat{ID: 7})
	if store.Notifiable(s, 7, 0, time.Time{}, time.Now()) {
		t.Fatal("zero event time must not notify")
	}
}
