package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderCache_ComputesOnMissCachesOnHit(t *testing.T) {
	c := newRenderCache()
	calls := 0
	render := func() []string {
		calls++
		return []string{"x"}
	}
	k := blockKey{photoID: 1, cols: 10}

	got1 := c.get(k, render)
	got2 := c.get(k, render)

	assert.Equal(t, 1, calls, "render should run once, then hit cache")
	assert.Equal(t, []string{"x"}, got1)
	assert.Equal(t, []string{"x"}, got2)
}

func TestRenderCache_EvictsLeastRecentlyUsedBeyondCap(t *testing.T) {
	c := newRenderCache()
	render := func() []string { return []string{"v"} }

	// Insert one more distinct key than the cache can hold.
	for i := 0; i < renderCacheCap+1; i++ {
		c.get(blockKey{photoID: int64(i), cols: 1}, render)
	}

	// Size stays bounded and the oldest (least-recently-used) key is evicted.
	assert.Equal(t, renderCacheCap, c.len())
	assert.False(t, c.contains(blockKey{photoID: 0, cols: 1}), "oldest key should be evicted")
	assert.True(t, c.contains(blockKey{photoID: renderCacheCap, cols: 1}), "newest key should be present")
}

func TestRenderCache_ResetForcesRecompute(t *testing.T) {
	c := newRenderCache()
	calls := 0
	render := func() []string {
		calls++
		return []string{"y"}
	}
	k := blockKey{photoID: 2, cols: 5}

	c.get(k, render)
	c.reset()
	c.get(k, render)

	assert.Equal(t, 2, calls, "reset should drop the cached entry")
}
