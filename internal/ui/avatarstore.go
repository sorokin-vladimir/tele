package ui

import (
	"image"

	lru "github.com/hashicorp/golang-lru/v2"
)

// avatarStoreCap bounds how many people's avatars are kept decoded in memory.
// A profile is opened one person at a time, so this only has to cover the
// handful someone looks at in a sitting; the disk cache behind it covers the
// rest.
const avatarStoreCap = 32

// avatarEntry is one person's decoded avatar and the id it was fetched under.
// The id is kept beside the image because it is the only thing that can say the
// remembered picture is still the current one.
type avatarEntry struct {
	avatarID int64
	img      image.Image
}

// avatarStore remembers decoded avatars by user id, so opening a profile a
// second time draws the picture at once instead of waiting for the full profile
// and a download.
//
// It is keyed by user rather than by avatar id on purpose: at the moment a
// profile opens, the avatar id is exactly what is not yet known — it arrives
// with the full profile. Asking "what did this person look like last time"
// is the only question that can be answered that early, and the id that comes
// back later either confirms the answer or replaces it (#223).
//
// Separate from the inline-image cache rather than sharing it: a scrolled chat
// evicting the faces of everyone you talk to is the same trade the disk caches
// were split to avoid, one layer up. Not safe for concurrent use — driven from
// the single bubbletea update goroutine.
type avatarStore struct {
	lru *lru.Cache[int64, avatarEntry]
}

// newAvatarStore returns an empty store. lru.New only errors on a non-positive
// size, which is a programming error here.
func newAvatarStore() *avatarStore {
	c, err := lru.New[int64, avatarEntry](avatarStoreCap)
	if err != nil {
		panic(err)
	}
	return &avatarStore{lru: c}
}

// Get returns what this person last looked like, and the id it was fetched
// under, marking the entry most-recently-used.
func (s *avatarStore) Get(userID int64) (avatarEntry, bool) {
	return s.lru.Get(userID)
}

// Add records a person's avatar under the id it was fetched with.
func (s *avatarStore) Add(userID, avatarID int64, img image.Image) {
	s.lru.Add(userID, avatarEntry{avatarID: avatarID, img: img})
}

// Remove drops a person's remembered avatar, so nothing stale is drawn after
// the bytes behind it turned out to be unusable.
func (s *avatarStore) Remove(userID int64) { s.lru.Remove(userID) }
