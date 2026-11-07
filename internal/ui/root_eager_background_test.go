package ui

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/priority"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// priorityOwner refuses every download and remembers, for each slot asked
// for, whether it was asked for as background work.
type priorityOwner struct {
	Owner
	background map[domain.MediaSlot]bool
}

func (o *priorityOwner) FetchMedia(ctx context.Context, _ int64, _ int, slot domain.MediaSlot) (string, error) {
	o.background[slot] = priority.IsBackground(ctx)
	return "", &telerr.Error{Kind: telerr.NotFound}
}

func (o *priorityOwner) SaveMedia(ctx context.Context, _ int64, _ int, slot domain.MediaSlot, _, _ string) (string, error) {
	o.background[slot] = priority.IsBackground(ctx)
	return "", &telerr.Error{Kind: telerr.NotFound}
}

// The full photo fetched ahead of time is nobody's wait, so it yields to the
// thumbnail the person is looking at (#212).
func TestEagerFullPhoto_IsBackgroundWorkAndTheThumbnailIsNot(t *testing.T) {
	m, _ := applyModel(t, "photos:\n  eager_full_quality: true\n")
	owner := &priorityOwner{background: map[domain.MediaSlot]bool{}}
	m.owner = owner

	runAll(m.pendingDownloadCmds([]domain.Message{{ID: 10, ChatID: 1,
		Photo: &domain.PhotoRef{ID: 321, FullThumbSize: "y"}}}))

	assert.Equal(t, map[domain.MediaSlot]bool{domain.PhotoThumb: false, domain.PhotoFull: true}, owner.background)
}
