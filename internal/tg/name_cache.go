package tg

import "sync"

// nameCache is a thread-safe userID -> display name cache. It bridges a gap in
// live updates: a supergroup UpdateNewChannelMessage sometimes omits the
// sender's User entity, leaving SenderName empty so the renderer falls back to
// "?" until the chat is refreshed (#161). Names observed in any update or in a
// history fetch are remembered here, so a later update missing the sender's
// entity can still resolve the author.
type nameCache struct {
	mu    sync.RWMutex
	names map[int64]string
}

func newNameCache() *nameCache {
	return &nameCache{names: make(map[int64]string)}
}

// put records id -> name. Zero id or empty name are ignored so a miss never
// overwrites a good entry with a blank.
func (c *nameCache) put(id int64, name string) {
	if id == 0 || name == "" {
		return
	}
	c.mu.Lock()
	c.names[id] = name
	c.mu.Unlock()
}

// get returns the cached name for id, or "" when unknown.
func (c *nameCache) get(id int64) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.names[id]
}
