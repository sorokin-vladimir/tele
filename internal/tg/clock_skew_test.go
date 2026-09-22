package tg

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/proto"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// skewReports records every value the tracker reports, in order.
type skewReports struct {
	mu  sync.Mutex
	got []time.Duration
}

func (r *skewReports) report(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, d)
}

func (r *skewReports) all() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Duration(nil), r.got...)
}

var skewTestNow = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

func newTestSkew(r *skewReports) *clockSkew {
	s := newClockSkew()
	s.now = func() time.Time { return skewTestNow }
	s.SetReport(r.report)
	return s
}

// logRejected writes the entry gotd writes when it drops a message whose id
// is outside its window, for a message the server created at serverTime. The
// text is gotd's (mtproto/read.go, v0.161.0); the stand in
// clock_skew_stand_test.go checks it against gotd itself.
func logRejected(log *zap.Logger, serverTime time.Time, how string) {
	id := proto.NewMessageID(serverTime, proto.MessageFromServer)
	err := fmt.Errorf("bad message id %d: created too far in %s: message rejected", int64(id), how)
	log.Warn("Ignoring rejected message", zap.NamedError("error", err))
}

func TestClockSkew_AheadIsReportedFromTheRejectedID(t *testing.T) {
	r := &skewReports{}
	log := watchClockSkew(zap.NewNop(), newTestSkew(r))

	logRejected(log, skewTestNow.Add(-8*time.Minute), "past")

	require.Len(t, r.all(), 1)
	assert.InDelta(t, float64(8*time.Minute), float64(r.all()[0]), float64(time.Second))
}

func TestClockSkew_BehindIsNegative(t *testing.T) {
	r := &skewReports{}
	log := watchClockSkew(zap.NewNop(), newTestSkew(r))

	logRejected(log, skewTestNow.Add(2*time.Minute), "future")

	require.Len(t, r.all(), 1)
	assert.InDelta(t, float64(-2*time.Minute), float64(r.all()[0]), float64(time.Second))
}

// gotd names its loggers and attaches fields as it goes; the watch has to
// survive both, or it sees nothing from the connection that matters.
func TestClockSkew_SurvivesNamedAndWith(t *testing.T) {
	r := &skewReports{}
	log := watchClockSkew(zap.NewNop(), newTestSkew(r)).Named("conn").With(zap.Int("dc", 2))

	logRejected(log, skewTestNow.Add(-8*time.Minute), "past")

	assert.Len(t, r.all(), 1)
}

// A message rejected for any other reason says nothing about the clock.
func TestClockSkew_OtherEntriesAreIgnored(t *testing.T) {
	r := &skewReports{}
	log := watchClockSkew(zap.NewNop(), newTestSkew(r))

	log.Warn("Ignoring rejected message",
		zap.NamedError("error", fmt.Errorf("duplicate or too low message id 42: message rejected")))
	log.Warn("Something else", zap.NamedError("error", fmt.Errorf("bad message id 42: created too far in past")))

	assert.Empty(t, r.all())
}

// Every server message is rejected while the clock is off, so the same skew is
// seen over and over. It is reported once, and again only when it moves by a
// minute or more.
func TestClockSkew_ReportedOnChangeOnly(t *testing.T) {
	r := &skewReports{}
	log := watchClockSkew(zap.NewNop(), newTestSkew(r))

	logRejected(log, skewTestNow.Add(-8*time.Minute), "past")
	logRejected(log, skewTestNow.Add(-8*time.Minute-10*time.Second), "past")
	logRejected(log, skewTestNow.Add(-10*time.Minute), "past")

	require.Len(t, r.all(), 2)
	assert.InDelta(t, float64(10*time.Minute), float64(r.all()[1]), float64(time.Second))
}

// The skew is over the moment a message gets through, and that is said once.
func TestClockSkew_PassedClearsOnce(t *testing.T) {
	r := &skewReports{}
	s := newTestSkew(r)
	log := watchClockSkew(zap.NewNop(), s)

	s.passed()
	logRejected(log, skewTestNow.Add(-8*time.Minute), "past")
	s.passed()
	s.passed()

	require.Len(t, r.all(), 2)
	assert.Equal(t, time.Duration(0), r.all()[1])
}

type okInvoker struct{}

func (okInvoker) Invoke(context.Context, bin.Encoder, bin.Decoder) error { return nil }

// A reply that came back is a message the clock check let through.
func TestClockSkew_SuccessfulRPCClears(t *testing.T) {
	r := &skewReports{}
	c := NewGotdClient(zap.NewNop(), nil, false, nil)
	c.skew = newTestSkew(r)
	logRejected(watchClockSkew(zap.NewNop(), c.skew), skewTestNow.Add(-8*time.Minute), "past")

	invoke := c.errorMiddleware().Handle(okInvoker{})
	require.NoError(t, invoke(context.Background(), &tg.HelpGetConfigRequest{}, &tg.Config{}))

	assert.Equal(t, []time.Duration{r.all()[0], 0}, r.all())
}

// So is an update Telegram pushed.
func TestClockSkew_ArrivedUpdateClears(t *testing.T) {
	r := &skewReports{}
	s := newTestSkew(r)
	logRejected(watchClockSkew(zap.NewNop(), s), skewTestNow.Add(-8*time.Minute), "past")
	next := &countingHandler{}

	require.NoError(t, s.arrivals(next).Handle(context.Background(), &tg.Updates{}))

	assert.Equal(t, 1, next.n)
	assert.Equal(t, time.Duration(0), r.all()[len(r.all())-1])
}

type countingHandler struct{ n int }

func (h *countingHandler) Handle(context.Context, tg.UpdatesClass) error {
	h.n++
	return nil
}
