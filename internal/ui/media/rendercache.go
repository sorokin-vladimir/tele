package media

import lru "github.com/hashicorp/golang-lru/v2"

// renderCacheCap bounds how many rendered results a single renderer keeps. Each
// entry is one photo's lines at one width; a long scrolling session touches many
// (photoID, cols) pairs, so an unbounded cache would grow monotonically. Matches
// the thumbnail image-cache order of magnitude (see imagecache).
const renderCacheCap = 256

// renderCache memoizes rendered terminal lines per blockKey, bounded by a
// fixed-capacity LRU so a long session stays memory-bounded. It is the single
// cache shared by BlockRenderer and KittyRenderer, which differ only in the
// miss-path render func they supply; keeping the policy in one place stops the
// two renderers from drifting apart. Not safe for concurrent use — callers drive
// it from the single bubbletea update/view goroutine.
type renderCache struct {
	lru *lru.Cache[blockKey, []string]
}

// newRenderCache returns an empty cache capped at renderCacheCap entries.
// lru.New only errors on a non-positive size, which is a programming error here.
func newRenderCache() *renderCache {
	c, err := lru.New[blockKey, []string](renderCacheCap)
	if err != nil {
		panic(err)
	}
	return &renderCache{lru: c}
}

// get returns the cached lines for key, computing them via render on a miss and
// storing the result before returning it. A hit or store marks key
// most-recently-used, evicting the least-recently-used entry when full.
func (c *renderCache) get(key blockKey, render func() []string) []string {
	if v, ok := c.lru.Get(key); ok {
		return v
	}
	v := render()
	c.lru.Add(key, v)
	return v
}

// reset drops every cached entry (call when the target width changes).
func (c *renderCache) reset() {
	c.lru.Purge()
}

// len returns the number of cached entries.
func (c *renderCache) len() int {
	return c.lru.Len()
}

// contains reports whether key is cached without affecting recency.
func (c *renderCache) contains(key blockKey) bool {
	return c.lru.Contains(key)
}
