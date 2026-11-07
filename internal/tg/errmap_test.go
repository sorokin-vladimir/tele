package tg

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/rpc"
	gotdtg "github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/telerr"
)

func testClient() *GotdClient {
	return &GotdClient{log: zap.NewNop(), traceLog: zap.NewNop()}
}

func TestClassifyTgErr(t *testing.T) {
	tests := []struct {
		name       string
		err        *tgerr.Error
		wantKind   telerr.Kind
		wantWait   time.Duration
		wantReason telerr.Reason
	}{
		{"flood wait", &tgerr.Error{Code: 420, Type: "FLOOD_WAIT", Argument: 30}, telerr.RateLimited, 30 * time.Second, ""},
		{"slowmode wait", &tgerr.Error{Code: 420, Type: "SLOWMODE_WAIT", Argument: 5}, telerr.RateLimited, 5 * time.Second, ""},
		{"auth key unregistered", &tgerr.Error{Code: 401, Type: "AUTH_KEY_UNREGISTERED"}, telerr.Unauthorized, 0, ""},
		{"auth key duplicated", &tgerr.Error{Code: 406, Type: "AUTH_KEY_DUPLICATED"}, telerr.Unauthorized, 0, ""},
		{"session revoked", &tgerr.Error{Code: 401, Type: "SESSION_REVOKED"}, telerr.Unauthorized, 0, ""},
		// A deleted account is a log out: logging in with the same number makes
		// a new account, or meets the ban below if there is one.
		{"user deleted", &tgerr.Error{Code: 401, Type: "USER_DEACTIVATED"}, telerr.Unauthorized, 0, ""},

		// A ban is a dead end rather than a log out: logging in again with the
		// same number is refused the same way (#254).
		{"account banned", &tgerr.Error{Code: 401, Type: "USER_DEACTIVATED_BAN"}, telerr.AccountBanned, 0, ""},
		{"number banned", &tgerr.Error{Code: 400, Type: "PHONE_NUMBER_BANNED"}, telerr.AccountBanned, 0, ""},

		// A 406 is not about the session as a family: most of them are refusals
		// raised while logging in (#254).
		{"406 for a mistyped number", &tgerr.Error{Code: 406, Type: "PHONE_NUMBER_INVALID"}, telerr.Internal, 0, ""},
		{"406 asking for an update", &tgerr.Error{Code: 406, Type: "UPDATE_APP_TO_LOGIN"}, telerr.Internal, 0, ""},

		// The app key, not the session. Neither code says so on its own, so the
		// type has to win first or a blocked key reads as an unexplained error.
		{"published app key", &tgerr.Error{Code: 406, Type: "API_ID_PUBLISHED_FLOOD"}, telerr.AppKeyBlocked, 0, ""},
		{"invalid app key", &tgerr.Error{Code: 400, Type: "API_ID_INVALID"}, telerr.AppKeyBlocked, 0, ""},

		{"forbidden by code", &tgerr.Error{Code: 403, Type: "CHAT_WRITE_FORBIDDEN"}, telerr.Forbidden, 0, ""},
		{"peer id invalid", &tgerr.Error{Code: 400, Type: "PEER_ID_INVALID"}, telerr.PeerNotFound, 0, ""},
		{"username nobody holds", &tgerr.Error{Code: 400, Type: "USERNAME_NOT_OCCUPIED"}, telerr.PeerNotFound, 0, ""},
		{"username that cannot be one", &tgerr.Error{Code: 400, Type: "USERNAME_INVALID"}, telerr.PeerNotFound, 0, ""},
		{"channel invalid", &tgerr.Error{Code: 400, Type: "CHANNEL_INVALID"}, telerr.PeerNotFound, 0, ""},
		{"forwards restricted", &tgerr.Error{Code: 400, Type: "CHAT_FORWARDS_RESTRICTED"}, telerr.Forbidden, 0, ""},
		{"send media forbidden by suffix", &tgerr.Error{Code: 400, Type: "CHAT_SEND_MEDIA_FORBIDDEN"}, telerr.Forbidden, 0, ""},
		{"file reference expired", &tgerr.Error{Code: 400, Type: "FILE_REFERENCE_EXPIRED"}, telerr.StaleReference, 0, ""},
		{"message id invalid", &tgerr.Error{Code: 400, Type: "MESSAGE_ID_INVALID"}, telerr.NotFound, 0, ""},
		{"server error", &tgerr.Error{Code: 500, Type: "INTERNAL"}, telerr.Network, 0, ""},
		{"unmapped", &tgerr.Error{Code: 400, Type: "SOMETHING_NEW"}, telerr.Internal, 0, ""},

		// The refusals: Telegram understood the request and would not take the
		// content. #224 is the first of these, and the whole class behaves alike.
		{"photo without a usable extension", &tgerr.Error{Code: 400, Type: "PHOTO_EXT_INVALID"},
			telerr.Rejected, 0, telerr.ReasonPhotoType},
		{"photo of impossible dimensions", &tgerr.Error{Code: 400, Type: "PHOTO_INVALID_DIMENSIONS"},
			telerr.Rejected, 0, telerr.ReasonPhotoDimensions},
		{"photo the server could not store", &tgerr.Error{Code: 400, Type: "PHOTO_SAVE_FILE_INVALID"},
			telerr.Rejected, 0, telerr.ReasonMediaUnreadable},
		{"image the server could not process", &tgerr.Error{Code: 400, Type: "IMAGE_PROCESS_FAILED"},
			telerr.Rejected, 0, telerr.ReasonMediaUnreadable},
		{"video the server could not read", &tgerr.Error{Code: 400, Type: "VIDEO_FILE_INVALID"},
			telerr.Rejected, 0, telerr.ReasonMediaUnreadable},
		{"media it will not carry", &tgerr.Error{Code: 400, Type: "MEDIA_INVALID"},
			telerr.Rejected, 0, telerr.ReasonMediaUnsupported},
		{"media that arrived empty", &tgerr.Error{Code: 400, Type: "MEDIA_EMPTY"},
			telerr.Rejected, 0, telerr.ReasonMediaUnsupported},
		{"nothing to send", &tgerr.Error{Code: 400, Type: "MESSAGE_EMPTY"},
			telerr.Rejected, 0, telerr.ReasonTextEmpty},
		{"text over the limit", &tgerr.Error{Code: 400, Type: "MESSAGE_TOO_LONG"},
			telerr.Rejected, 0, telerr.ReasonTextTooLong},
		{"markup over the limit", &tgerr.Error{Code: 400, Type: "ENTITIES_TOO_LONG"},
			telerr.Rejected, 0, telerr.ReasonMarkupTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, reason, wait := classifyTgErr(tt.err)
			assert.Equal(t, tt.wantKind, kind)
			assert.Equal(t, tt.wantWait, wait)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}

// The point of keeping the list explicit. An unrecognised refusal must stay
// Internal, because Internal is the admission that we do not know what happened
// and it is what the unmapped-error warning is watching for. Sorting unknown
// 400s into Rejected would have us telling a person, with confidence, that
// Telegram refused their content — about an error we cannot read.
func TestClassifyTgErr_AnUnknownRefusalIsNotRejected(t *testing.T) {
	kind, reason, _ := classifyTgErr(&tgerr.Error{Code: 400, Type: "SOME_NEW_REFUSAL"})

	assert.Equal(t, telerr.Internal, kind)
	assert.Empty(t, reason)
}

func TestMapError_ARefusalCarriesItsReasonAndItsEvidence(t *testing.T) {
	err := testClient().mapError("messages.sendMedia", &tgerr.Error{Code: 400, Type: "PHOTO_EXT_INVALID"})

	e, ok := telerr.As(err)
	require.True(t, ok)
	assert.Equal(t, telerr.Rejected, e.Kind)
	assert.Equal(t, telerr.ReasonPhotoType, e.Reason)
	assert.Equal(t, "PHOTO_EXT_INVALID", e.Detail, "the raw type stays for the logs")
	assert.False(t, e.Transient)
}

// Every log out says how it happened: that is what the person is told on the
// way back to the login (#297).
func TestMapError_ALogOutSaysHowItHappened(t *testing.T) {
	for typ, want := range map[string]telerr.LogOut{
		"AUTH_KEY_UNREGISTERED": telerr.LogOutElsewhere,
		"SESSION_REVOKED":       telerr.LogOutElsewhere,
		"SESSION_EXPIRED":       telerr.LogOutElsewhere,
		"USER_DEACTIVATED":      telerr.LogOutDeleted,
	} {
		e, ok := telerr.As(testClient().mapError("users.getUsers", tgerr.New(401, typ)))
		require.True(t, ok)
		assert.Equal(t, want, e.LogOut, typ)
	}
	e, _ := telerr.As(testClient().mapError("users.getUsers", tgerr.New(406, "AUTH_KEY_DUPLICATED")))
	assert.Equal(t, telerr.LogOutKeyDropped, e.LogOut)

	e, _ = telerr.As(testClient().mapError("messages.sendMessage", tgerr.New(403, "CHAT_WRITE_FORBIDDEN")))
	assert.Empty(t, e.LogOut, "only a log out carries one")
}

func TestMapError_NilStaysNil(t *testing.T) {
	assert.NoError(t, testClient().mapError("messages.sendMessage", nil))
}

func TestMapError_SetsOpAndDetail(t *testing.T) {
	err := testClient().mapError("messages.sendMessage", &tgerr.Error{Code: 400, Type: "PEER_ID_INVALID"})
	e, ok := telerr.As(err)
	require.True(t, ok)
	assert.Equal(t, telerr.PeerNotFound, e.Kind)
	assert.Equal(t, "messages.sendMessage", e.Op)
	assert.Equal(t, "PEER_ID_INVALID", e.Detail)
}

func TestMapError_ServerErrorIsTransient(t *testing.T) {
	err := testClient().mapError("messages.getHistory", &tgerr.Error{Code: 500, Type: "INTERNAL"})
	e, ok := telerr.As(err)
	require.True(t, ok)
	assert.True(t, e.Transient)
}

func TestMapError_NetErrorIsTransientNetwork(t *testing.T) {
	var netErr net.Error = &net.DNSError{Err: "no such host", IsTemporary: true}
	err := testClient().mapError("messages.getHistory", netErr)
	e, ok := telerr.As(err)
	require.True(t, ok)
	assert.Equal(t, telerr.Network, e.Kind)
	assert.True(t, e.Transient)
}

func TestMapError_ContextErrorsPassThrough(t *testing.T) {
	c := testClient()
	for _, ctxErr := range []error{context.Canceled, context.DeadlineExceeded} {
		mapped := c.mapError("op", ctxErr)
		assert.ErrorIs(t, mapped, ctxErr)
		_, isDomain := telerr.As(mapped)
		assert.False(t, isDomain, "cancellation is control flow, not a domain failure")
	}
}

func TestMapError_AlreadyMappedPassesThrough(t *testing.T) {
	already := &telerr.Error{Kind: telerr.NotFound, Op: "first"}
	assert.Same(t, error(already), testClient().mapError("second", already))
}

func TestMapError_PlainErrorIsInternalWithText(t *testing.T) {
	err := testClient().mapError("op", errors.New("boom"))
	e, ok := telerr.As(err)
	require.True(t, ok)
	assert.Equal(t, telerr.Internal, e.Kind)
	assert.Equal(t, "boom", e.Detail)
}

// fakeInvoker stands in for the gotd RPC chain.
type fakeInvoker struct{ err error }

func (f *fakeInvoker) Invoke(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	return f.err
}

func TestErrorMiddleware_MapsAndNamesTheOperation(t *testing.T) {
	c := testClient()
	invoker := c.errorMiddleware().Handle(&fakeInvoker{err: &tgerr.Error{Code: 400, Type: "PEER_ID_INVALID"}})

	err := invoker.Invoke(context.Background(), &gotdtg.MessagesSendMessageRequest{}, nil)

	e, ok := telerr.As(err)
	require.True(t, ok)
	assert.Equal(t, telerr.PeerNotFound, e.Kind)
	assert.Equal(t, "messages.sendMessage", e.Op)
}

func TestErrorMiddleware_SuccessStaysNil(t *testing.T) {
	c := testClient()
	invoker := c.errorMiddleware().Handle(&fakeInvoker{})
	assert.NoError(t, invoker.Invoke(context.Background(), &gotdtg.MessagesSendMessageRequest{}, nil))
}

func TestAcquireAPI_NotConnectedIsTransientNetwork(t *testing.T) {
	_, err := testClient().acquireAPI()
	e, ok := telerr.As(err)
	require.True(t, ok)
	assert.Equal(t, telerr.Network, e.Kind)
	assert.True(t, e.Transient)
}

// The invariant that keeps gotd working: it recognises its own errors through
// the unwrap chain, so mapping must not hide them.
func TestMapError_PreservesTgerrForGotd(t *testing.T) {
	mapped := testClient().mapError("op", &tgerr.Error{Code: 420, Type: "FLOOD_WAIT", Argument: 3})
	assert.True(t, tgerr.Is(mapped, "FLOOD_WAIT"))

	var te *tgerr.Error
	require.True(t, errors.As(mapped, &te))
	assert.Equal(t, 420, te.Code)
}

// gotd reports a request the server never acknowledged as RetryLimitReachedErr.
// Its own documentation calls it exactly that — "server does not acknowledge
// request after multiple retries" — which is a transient transport failure, not
// a bug. Left unmapped it became Internal, which is terminal: an offline send
// was marked failed forever instead of waiting for the connection (#193).
func TestMapError_UnacknowledgedRequestIsTransientNetwork(t *testing.T) {
	c := testClient()

	got := c.mapError("messages.sendMessage", &rpc.RetryLimitReachedErr{Retries: 5})

	e, ok := telerr.As(got)
	require.True(t, ok)
	assert.Equal(t, telerr.Network, e.Kind)
	assert.True(t, e.Transient, "an unacknowledged request is worth repeating")
}

func TestMapError_AClosedEngineIsTransientNetwork(t *testing.T) {
	c := testClient()

	got := c.mapError("messages.sendMessage", rpc.ErrEngineClosed)

	e, ok := telerr.As(got)
	require.True(t, ok)
	assert.Equal(t, telerr.Network, e.Kind)
	assert.True(t, e.Transient)
}

// Wrapped the way gotd actually delivers it, through rpcDoRequest.
func TestMapError_UnacknowledgedRequestThroughAWrapChain(t *testing.T) {
	c := testClient()
	wrapped := fmt.Errorf("rpcDoRequest: %w", &rpc.RetryLimitReachedErr{Retries: 5})

	got := c.mapError("messages.sendMessage", wrapped)

	assert.Equal(t, telerr.Network, telerr.Of(got))
}
