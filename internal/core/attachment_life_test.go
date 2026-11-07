package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// heldSearch is a connection whose contact search waits until it is let go or
// its call is called off, so a test can end an account while it runs.
type heldSearch struct {
	internaltg.Client
	started chan struct{}
}

func (h *heldSearch) Connect(context.Context, *internaltg.AuthFlow, chan<- struct{}, func(domain.User)) error {
	return nil
}

func (h *heldSearch) Updates() <-chan store.Event { return nil }

func (h *heldSearch) SearchContacts(ctx context.Context, _ string, _ int) ([]domain.Chat, error) {
	close(h.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

// A call still running when its account ends is answered as ended, not with
// whatever the stop broke under it: the account it was meant for is gone, and
// the next one may be somebody else's (#309).
func TestAttachment_ACallInFlightWhenTheAccountEndsIsAnsweredAsEnded(t *testing.T) {
	c := &heldSearch{started: make(chan struct{})}
	o, _ := newOwnerWithClient(t, c)
	a := attach(o)
	done := make(chan error, 1)
	go func() {
		_, err := a.SearchContacts(context.Background(), "ada", 10)
		done <- err
	}()
	<-c.started

	go a.Leave(AccountEnded{Reason: EndLoggedOutElsewhere})

	select {
	case err := <-done:
		assert.Equal(t, telerr.AccountEnded, telerr.Of(err))
	case <-time.After(time.Second):
		require.Fail(t, "the call outlived its account")
	}
}

// Between accounts there is no owner to ask, and a call says so rather than
// waiting for one.
func TestAttachment_ACallBetweenAccountsIsAnsweredAsEnded(t *testing.T) {
	a := NewAttachment()

	err := a.MarkRead(context.Background(), domain.HistoryKey{ChatID: 1}, 0)

	assert.Equal(t, telerr.AccountEnded, telerr.Of(err))
}

// The client hears that the account ended, and nothing of that owner after it:
// the stream is one and ordered, so whatever follows the news is the next
// account's.
func TestAttachment_NothingOfAnEndedOwnerFollowsTheNews(t *testing.T) {
	o, _, st := newTestOwner(t)
	st.SetChat(domain.Chat{ID: 1, Title: "Ada"})
	a := attach(o)
	a.Subscribe(project.ChatListWindow{Limit: 10})
	_, _ = recvDelta(t, a)

	a.Leave(AccountEnded{Reason: EndLoggedOutHere, Discarded: 2})
	st.SetChat(domain.Chat{ID: 1, Title: "Grace"})
	o.Refresh()

	evs := queued(a)
	require.Len(t, evs, 1, "the news, and nothing of the owner after it")
	assert.Equal(t, AccountEnded{Reason: EndLoggedOutHere, Discarded: 2}, evs[0])
}

// A client's next screen subscribes as soon as the account ended, before the
// next one is open. The subscription waits for the next owner and is made
// there, under the id it was given; ids go on growing across owners.
func TestAttachment_ASubscriptionMadeBetweenAccountsIsMadeAtTheNext(t *testing.T) {
	first, _, _ := newTestOwner(t)
	a := attach(first)
	before := a.Subscribe(project.ChatListWindow{Limit: 10})
	_, _ = recvDelta(t, a)
	a.Leave(AccountEnded{Reason: EndLoggedOutHere})
	queued(a)

	id := a.Subscribe(project.AccountWindow{})
	next, _, _ := newTestOwner(t)
	a.Join(next)

	d, ok := recvDelta(t, a)
	require.True(t, ok, "the next owner answers the subscription")
	assert.Equal(t, id, d.Sub)
	require.NotNil(t, d.Account)
	assert.Greater(t, id, before, "an id is never handed out twice")
}

// The next owner hears the client: its focus and its subscriptions are there,
// not at the owner that ended.
func TestAttachment_TheNextOwnerIsTheOneAsked(t *testing.T) {
	first, _, _ := newTestOwner(t)
	a := attach(first)
	a.Leave(AccountEnded{Reason: EndLoggedOutHere})
	next, _, _ := newTestOwner(t)
	a.Join(next)

	a.SetFocus(domain.HistoryKey{ChatID: 7})

	assert.True(t, next.focus.focused(7))
	assert.False(t, first.focus.focused(7))
}

// A client that went away is not waited on: the host tells it nothing more,
// and does not hang trying.
func TestAttachment_TheHostDoesNotWaitOnAClientThatWent(t *testing.T) {
	a := NewAttachment()
	for len(a.stream) < cap(a.stream) {
		a.stream <- struct{}{}
	}
	a.Detach()

	done := make(chan struct{})
	go func() {
		a.Tell(HostFailure{})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "the host hung on a client that is gone")
	}
}
