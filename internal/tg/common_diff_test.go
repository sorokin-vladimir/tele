package tg

import (
	"context"
	"errors"
	"testing"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeUpdatesAPI stands in for the raw client behind the updates manager. Only
// the common-state difference call has behavior; the other two satisfy the
// interface.
type fakeUpdatesAPI struct {
	// commonDiff answers the common-state difference call. Unset means an
	// account with nothing pending.
	commonDiff tg.UpdatesDifferenceClass
	commonErr  error
}

func (f *fakeUpdatesAPI) UpdatesGetState(context.Context) (*tg.UpdatesState, error) {
	return &tg.UpdatesState{}, nil
}

func (f *fakeUpdatesAPI) UpdatesGetDifference(
	context.Context, *tg.UpdatesGetDifferenceRequest,
) (tg.UpdatesDifferenceClass, error) {
	if f.commonErr != nil {
		return nil, f.commonErr
	}
	if f.commonDiff == nil {
		return &tg.UpdatesDifferenceEmpty{}, nil
	}
	return f.commonDiff, nil
}

func (f *fakeUpdatesAPI) UpdatesGetChannelDifference(
	context.Context, *tg.UpdatesGetChannelDifferenceRequest,
) (tg.UpdatesChannelDifferenceClass, error) {
	return &tg.UpdatesChannelDifferenceEmpty{}, nil
}

// captureHandler stands in for the dispatcher and records what it was handed.
type captureHandler struct {
	got []tg.UpdatesClass
	err error
}

var _ telegram.UpdateHandler = (*captureHandler)(nil)

func (h *captureHandler) Handle(_ context.Context, u tg.UpdatesClass) error {
	h.got = append(h.got, u)
	return h.err
}

func callCommonDiff(
	t *testing.T,
	diff tg.UpdatesDifferenceClass,
	h *captureHandler,
) tg.UpdatesDifferenceClass {
	t.Helper()
	raw := &fakeUpdatesAPI{commonDiff: diff}
	got, err := newCommonDiffAPI(raw, h, zap.NewNop()).
		UpdatesGetDifference(context.Background(), &tg.UpdatesGetDifferenceRequest{Pts: 101})
	require.NoError(t, err)
	return got
}

func editUpdate(msgID, pts int, text string) *tg.UpdateEditMessage {
	msg := &tg.Message{ID: msgID, Message: text, PeerID: &tg.PeerUser{UserID: 10}}
	msg.FromID = &tg.PeerUser{UserID: 10}
	return &tg.UpdateEditMessage{Message: msg, Pts: pts, PtsCount: 1}
}

func TestCommonDiffAPI_DeliversTheUpdatesADifferenceRecovered(t *testing.T) {
	h := &captureHandler{}
	diff := &tg.UpdatesDifference{
		OtherUpdates: []tg.UpdateClass{editUpdate(5, 130, "the whole answer")},
		Users:        []tg.UserClass{&tg.User{ID: 10, AccessHash: 42}},
		Chats:        []tg.ChatClass{&tg.Chat{ID: 7}},
		State:        tg.UpdatesState{Pts: 130},
	}

	got := callCommonDiff(t, diff, h)

	require.Len(t, h.got, 1)
	comb, ok := h.got[0].(*tg.UpdatesCombined)
	require.True(t, ok, "the handler needs the users and chats, so it gets a combined envelope")
	assert.Equal(t, diff.OtherUpdates, comb.Updates)
	assert.Equal(t, diff.Users, comb.Users, "peers come with the updates that name them")
	assert.Equal(t, diff.Chats, comb.Chats)

	// The manager still gets its difference, unchanged.
	assert.Equal(t, diff, got)
}

func TestCommonDiffAPI_DeliversFromASlicedDifferenceToo(t *testing.T) {
	h := &captureHandler{}
	diff := &tg.UpdatesDifferenceSlice{
		OtherUpdates:      []tg.UpdateClass{editUpdate(5, 120, "half of it")},
		Users:             []tg.UserClass{&tg.User{ID: 10, AccessHash: 42}},
		IntermediateState: tg.UpdatesState{Pts: 120},
	}

	got := callCommonDiff(t, diff, h)

	require.Len(t, h.got, 1)
	assert.Equal(t, diff, got)
}

// New messages are not in OtherUpdates: the manager hands those to the handler
// itself, by a path that does not drop them. Delivering them again here would
// be duplicate traffic bought for nothing.
func TestCommonDiffAPI_SaysNothingWhenThereIsNothingToRecover(t *testing.T) {
	h := &captureHandler{}

	callCommonDiff(t, &tg.UpdatesDifferenceEmpty{Date: 100, Seq: 3}, h)
	callCommonDiff(t, &tg.UpdatesDifference{
		NewMessages: []tg.MessageClass{&tg.Message{ID: 9}},
		State:       tg.UpdatesState{Pts: 130},
	}, h)

	assert.Empty(t, h.got)
}

// A difference too long is the manager's own signal that a range is gone for
// good. It carries no updates to deliver, and the gap recording behind it is
// not ours to touch here (#262).
func TestCommonDiffAPI_LeavesATooLongDifferenceAlone(t *testing.T) {
	h := &captureHandler{}

	got := callCommonDiff(t, &tg.UpdatesDifferenceTooLong{Pts: 900}, h)

	assert.Empty(t, h.got)
	assert.Equal(t, &tg.UpdatesDifferenceTooLong{Pts: 900}, got)
}

// The difference must reach the manager whatever a handler does with its copy:
// the manager's position depends on it, and a handler that failed is a dropped
// event, not a broken catch-up.
func TestCommonDiffAPI_AHandlerFailureDoesNotBreakTheCatchUp(t *testing.T) {
	h := &captureHandler{err: errors.New("handler is unhappy")}
	diff := &tg.UpdatesDifference{
		OtherUpdates: []tg.UpdateClass{editUpdate(5, 130, "the whole answer")},
		State:        tg.UpdatesState{Pts: 130},
	}

	got := callCommonDiff(t, diff, h)

	require.Len(t, h.got, 1)
	assert.Equal(t, diff, got)
}

func TestCommonDiffAPI_AFailedDifferenceIsPassedBackAsItIs(t *testing.T) {
	h := &captureHandler{}
	boom := errors.New("no difference for you")
	raw := &fakeUpdatesAPI{commonErr: boom}

	_, err := newCommonDiffAPI(raw, h, zap.NewNop()).
		UpdatesGetDifference(context.Background(), &tg.UpdatesGetDifferenceRequest{Pts: 101})

	require.ErrorIs(t, err, boom)
	assert.Empty(t, h.got)
}
