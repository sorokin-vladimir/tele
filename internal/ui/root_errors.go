package ui

import (
	"context"
	"errors"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// errText renders any error for the status bar. Domain kinds get a phrase and a
// severity derived from the kind, so the same failure never reads as Info in
// one place and Warning in another. Local failures keep their own text, which
// is both useful and safe to show. ok is false when nothing should appear.
//
// action names the attempt as a noun phrase, without the word "failed":
// "photo download", "mark read", "load history".
func errText(action string, err error) (string, components.Severity, bool) {
	if err == nil {
		return "", 0, false
	}
	// Checked before the kind: a context error is never wrapped, so it would
	// otherwise be reported as Internal and shown to a user who simply pressed
	// Esc.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "", 0, false
	}

	e, ok := telerr.As(err)
	if !ok {
		return action + ": " + err.Error(), components.SeverityWarning, true
	}

	switch e.Kind {
	case telerr.Unauthorized:
		return action + ": session expired, log in again", components.SeverityError, true
	case telerr.AppKeyBlocked:
		// Both remedies, on their own lines: which one is easier is the
		// person's to judge, and neither fits beside the cause on a narrow
		// terminal. Toasts wrap and the chat pane centres the block, so the
		// line breaks survive both surfaces.
		return action + ": app key blocked by Telegram\n" +
			"set your own telegram.api_id in the config,\n" +
			"or install an official build (see the README)", components.SeverityError, true
	case telerr.RateLimited:
		return action + ": too fast, retry in " + formatWait(e.RetryAfter), components.SeverityWarning, true
	case telerr.PeerNotFound:
		return action + ": chat unavailable", components.SeverityWarning, true
	case telerr.Forbidden:
		return action + ": not allowed in this chat", components.SeverityWarning, true
	case telerr.NotFound:
		return action + ": message no longer exists", components.SeverityWarning, true
	case telerr.StaleReference:
		return action + ": media reference expired", components.SeverityWarning, true
	case telerr.Network:
		return action + ": no connection", components.SeverityWarning, true
	case telerr.Rejected:
		// The same phrase the status bar uses when the cursor rests on the
		// entry, so the toast that announces it and the reminder that outlives
		// it do not describe one failure two ways (#224).
		return action + ": " + components.RejectionReason(e.Reason), components.SeverityWarning, true
	default:
		// Internal keeps the raw Telegram type: "internal error" on its own is
		// useless to someone about to file an issue, and an error type is not
		// private data.
		detail := e.Detail
		if detail == "" {
			detail = "internal error"
		}
		return action + ": " + detail, components.SeverityError, true
	}
}

// errStatus is errText for the common case of a plain status message. It
// returns nil when nothing should be shown, which bubbletea treats as a no-op.
func errStatus(action string, err error) tea.Msg {
	text, sev, ok := errText(action, err)
	if !ok {
		return nil
	}
	return StatusErrMsg{Text: text, Sev: sev}
}

// errStatusBackground is errStatus for work nobody asked for: an inline
// preview, the eager full-quality prefetch, the read pointer following the
// viewport. Two kinds are swallowed, both because they repair themselves and
// neither because they are unimportant.
//
// An expired file reference is the owner's business — it refreshes the
// reference, logs whatever it could not repair and publishes the fresh one,
// which brings the fetch back through the window. A chat reopened from disk
// expires several at once, which is a stack of identical toasts over media that
// then appears anyway.
//
// A transient network failure means the connection is down, and background work
// is re-attempted as soon as it returns. Marking read fires on every cursor
// move, so an offline session otherwise fills with a toast per keypress about a
// thing the reader cannot act on and did not ask for.
func errStatusBackground(action string, err error) tea.Msg {
	if telerr.Of(err) == telerr.StaleReference {
		return nil
	}
	if e, ok := telerr.As(err); ok && e.Kind == telerr.Network && e.Transient {
		return nil
	}
	return errStatus(action, err)
}

// formatWait renders a rate-limit wait the way a person would say it.
func formatWait(d time.Duration) string {
	switch {
	case d <= 0:
		return "a moment"
	case d < time.Minute:
		return strconv.Itoa(int(d.Round(time.Second).Seconds())) + "s"
	default:
		return strconv.Itoa(int(d.Round(time.Minute).Minutes())) + "m"
	}
}
