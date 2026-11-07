package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// What ends an account is told to the person on the way back to the login, so
// it is named from a closed list rather than read off Telegram's errors (#297).
func TestEndReasonOf_ALogOutEndsTheAccount(t *testing.T) {
	for logOut, want := range map[telerr.LogOut]EndReason{
		telerr.LogOutHere:       EndLoggedOutHere,
		telerr.LogOutElsewhere:  EndLoggedOutElsewhere,
		telerr.LogOutDeleted:    EndAccountDeleted,
		telerr.LogOutKeyDropped: EndKeyInvalidated,
	} {
		got, ok := EndReasonOf(&telerr.Error{Kind: telerr.Unauthorized, LogOut: logOut})
		assert.True(t, ok, logOut)
		assert.Equal(t, want, got, logOut)
	}
}

// A connection that ended for any other reason leaves the account as it is: a
// dropped network is not a log out, and nothing of the account is removed.
func TestEndReasonOf_AnythingElseEndsNothing(t *testing.T) {
	for _, err := range []error{
		nil,
		&telerr.Error{Kind: telerr.Network, Transient: true},
		&telerr.Error{Kind: telerr.AccountBanned},
		&telerr.Error{Kind: telerr.Unauthorized},
		assert.AnError,
	} {
		_, ok := EndReasonOf(err)
		assert.False(t, ok, "%v", err)
	}
}

// A sent message waits in the queue only until its update arrives, and is not
// lost with the account: it is already Telegram's.
func TestUnsentCount_CountsWhatHasNotGoneOut(t *testing.T) {
	o, _, _ := newTestOwner(t)
	assert.Zero(t, o.UnsentCount(), "no send queue, nothing unsent")

	q := newOutboxStore(t)
	o.SetOutbox(q)
	for _, ref := range []string{"queued", "failed", "sent"} {
		_, _, err := q.Add(domain.OutboxEntry{Ref: ref, ChatID: 1, State: domain.OutboxQueued})
		require.NoError(t, err)
	}
	failed, _ := q.Get("failed")
	failed.State = domain.OutboxFailed
	require.NoError(t, q.Update(failed))
	sent, _ := q.Get("sent")
	sent.State, sent.SentMsgIDs = domain.OutboxSending, []int{7}
	require.NoError(t, q.Update(sent))

	assert.Equal(t, 2, o.UnsentCount())
}

// The confirmation names who is being logged out and what is lost with them,
// both of which only the owner knows.
func TestLogOutPreview_NamesTheAccountAndWhatGoesWithIt(t *testing.T) {
	o, _, _ := newTestOwner(t)
	o.onAuth(domain.User{ID: 42, Username: "ada"})

	p := o.LogOutPreview()

	assert.Equal(t, "@ada", p.Name)
	assert.Zero(t, p.Unsent)
}

func TestLogOutPreview_AnAccountWithNoUsername(t *testing.T) {
	o, _, _ := newTestOwner(t)
	o.onAuth(domain.User{ID: 42})

	assert.Equal(t, "this account", o.LogOutPreview().Name)
}

func (s *stubClient) LogOut(_ context.Context, tellTelegram bool) error {
	s.loggedOut = append(s.loggedOut, tellTelegram)
	return s.err
}

// A client can ask for the account to end when nothing ended the connection
// for it: a banned account's connection is already over, and the person leaves
// it to log in with another number (#297). The request is the host's to hear,
// so it goes through the attachment, not the owner (#309).
func TestEndAccount_IsHandedToTheHost(t *testing.T) {
	a := NewAttachment()

	a.EndAccount(EndBanned)
	a.EndAccount(EndBanned) // a second ask while one waits does not block

	assert.Equal(t, EndBanned, <-a.EndRequests())
}

// Logging out is the connection's to carry out: the owner hands it on.
func TestLogOut_IsHandedToTheConnection(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)

	require.NoError(t, o.LogOut(context.Background(), false))

	assert.Equal(t, []bool{false}, c.loggedOut)
}
