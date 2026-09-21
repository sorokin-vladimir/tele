package ui

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func TestErrText_KindToTextAndSeverity(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantTxt string
		wantSev components.Severity
	}{
		{
			"unauthorized",
			&telerr.Error{Kind: telerr.Unauthorized},
			"mark read: session expired, log in again",
			components.SeverityError,
		},
		{
			"a blocked app key names the cause and both remedies",
			&telerr.Error{Kind: telerr.AppKeyBlocked, Detail: "API_ID_PUBLISHED_FLOOD"},
			"mark read: app key blocked by Telegram\n" +
				"set your own telegram.api_id in the config,\n" +
				"or install an official build (see the README)",
			components.SeverityError,
		},
		{
			"rate limited in minutes",
			&telerr.Error{Kind: telerr.RateLimited, RetryAfter: 12 * time.Minute},
			"mark read: too fast, retry in 12m",
			components.SeverityWarning,
		},
		{
			"rate limited in seconds",
			&telerr.Error{Kind: telerr.RateLimited, RetryAfter: 30 * time.Second},
			"mark read: too fast, retry in 30s",
			components.SeverityWarning,
		},
		{
			"a refusal names what was wrong rather than the protocol type",
			&telerr.Error{Kind: telerr.Rejected, Reason: telerr.ReasonPhotoType, Detail: "PHOTO_EXT_INVALID"},
			"mark read: not a file Telegram accepts as a photo",
			components.SeverityWarning,
		},
		{
			"a refusal with no phrase for it stays honest",
			&telerr.Error{Kind: telerr.Rejected, Detail: "SOME_NEW_REFUSAL"},
			"mark read: Telegram would not accept it",
			components.SeverityWarning,
		},
		{
			"peer not found",
			&telerr.Error{Kind: telerr.PeerNotFound},
			"mark read: chat unavailable",
			components.SeverityWarning,
		},
		{
			"forbidden",
			&telerr.Error{Kind: telerr.Forbidden},
			"mark read: not allowed in this chat",
			components.SeverityWarning,
		},
		{
			"not found",
			&telerr.Error{Kind: telerr.NotFound},
			"mark read: message no longer exists",
			components.SeverityWarning,
		},
		{
			"stale reference",
			&telerr.Error{Kind: telerr.StaleReference},
			"mark read: media reference expired",
			components.SeverityWarning,
		},
		{
			"network",
			&telerr.Error{Kind: telerr.Network},
			"mark read: no connection",
			components.SeverityWarning,
		},
		{
			"internal shows the raw type",
			&telerr.Error{Kind: telerr.Internal, Detail: "SOMETHING_NEW"},
			"mark read: SOMETHING_NEW",
			components.SeverityError,
		},
		{
			"internal without a detail still says something",
			&telerr.Error{Kind: telerr.Internal},
			"mark read: internal error",
			components.SeverityError,
		},
		{
			"local error keeps its text",
			errors.New("permission denied"),
			"mark read: permission denied",
			components.SeverityWarning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, sev, ok := errText("mark read", tt.err)
			require.True(t, ok)
			assert.Equal(t, tt.wantTxt, text)
			assert.Equal(t, tt.wantSev, sev)
		})
	}
}

func TestErrText_WrappedDomainErrorIsRecognised(t *testing.T) {
	err := fmt.Errorf("download photo 42: %w", &telerr.Error{Kind: telerr.Network})
	text, _, ok := errText("photo download", err)
	require.True(t, ok)
	assert.Equal(t, "photo download: no connection", text)
}

func TestErrText_CancelledShowsNothing(t *testing.T) {
	_, _, ok := errText("photo download", context.Canceled)
	assert.False(t, ok, "the user asked for the stop; do not report it as a failure")

	_, _, ok = errText("photo download", fmt.Errorf("download: %w", context.Canceled))
	assert.False(t, ok)

	_, _, ok = errText("photo download", context.DeadlineExceeded)
	assert.False(t, ok)
}

func TestErrText_NilShowsNothing(t *testing.T) {
	_, _, ok := errText("photo download", nil)
	assert.False(t, ok)
}

func TestErrStatus_BuildsStatusErrMsg(t *testing.T) {
	msg := errStatus("mark read", &telerr.Error{Kind: telerr.Network})
	status, ok := msg.(StatusErrMsg)
	require.True(t, ok)
	assert.Equal(t, "mark read: no connection", status.Text)
	assert.Equal(t, components.SeverityWarning, status.Sev)
}

func TestErrStatus_CancelledYieldsNoMessage(t *testing.T) {
	assert.Nil(t, errStatus("mark read", context.Canceled))
}

func TestMarkReadFailure_UsesTheRenderer(t *testing.T) {
	msg := errStatus("mark read", &telerr.Error{Kind: telerr.Forbidden})
	status, ok := msg.(StatusErrMsg)
	require.True(t, ok)
	assert.Equal(t, "mark read: not allowed in this chat", status.Text)
	// Severity now follows the kind rather than the call site: this used to be
	// SeverityInfo because it was typed by hand.
	assert.Equal(t, components.SeverityWarning, status.Sev)
}

func TestHandleFileDownloadDone_EmptyTextRaisesNoToast(t *testing.T) {
	m := NewRootModel(nil, 50, false)
	m2, _ := m.handleFileDownloadDone(fileDownloadDoneMsg{serial: 1, text: ""})
	assert.True(t, m2.toasts.Empty(), "a cancelled download must not raise an empty toast")
}

func TestHandleFileDownloadDone_TextStillRaisesToast(t *testing.T) {
	m := NewRootModel(nil, 50, false)
	m2, _ := m.handleFileDownloadDone(fileDownloadDoneMsg{serial: 1, text: "Saved to /tmp/x.jpg"})
	assert.False(t, m2.toasts.Empty())
}

// Background work that failed because the connection is down needs no toast: it
// repeats itself when the connection returns. Marking read runs on nearly every
// keypress, so saying it each time is how an offline session fills with toasts
// nobody can act on (#193).
func TestErrStatusBackground_SwallowsATransientNetworkFailure(t *testing.T) {
	got := errStatusBackground("mark read", &telerr.Error{Kind: telerr.Network, Transient: true})

	assert.Nil(t, got)
}

func TestErrStatusBackground_ReportsAFailureThatWillNotRepair(t *testing.T) {
	got := errStatusBackground("mark read", &telerr.Error{Kind: telerr.Forbidden})

	assert.NotNil(t, got, "a refusal is not going to fix itself")
}

func TestErrStatusBackground_ReportsAPermanentNetworkFailure(t *testing.T) {
	got := errStatusBackground("mark read", &telerr.Error{Kind: telerr.Network})

	assert.NotNil(t, got)
}
