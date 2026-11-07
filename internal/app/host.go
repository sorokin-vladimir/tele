package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/accountstate"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// The host's side of an account's life (#297). The host opens an account and
// runs it, and seats its clients at its owner; when the account ends with a
// log out, the host tells the clients, stops the account whole, removes it
// from disk, opens the next one and seats the clients there. Nothing of an
// account lives in the host, which is what lets the next one start with
// nothing of the last. The clients outlive the accounts as the host does, and
// draw each one themselves (#309).

// current is the account running now.
func (h *Host) current() *account {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.acct
}

// openNext opens the next account over the state directory, makes it the
// current one and seats the clients at its owner. Its scratch files live in a
// directory of its own, so they go with it rather than pass to whoever logs in
// next.
func (h *Host) openNext() (*account, error) {
	h.mu.Lock()
	h.opened++
	n := h.opened
	h.mu.Unlock()

	cfg := h.cfg()
	tmpDir := ""
	if h.tmpDir != "" {
		tmpDir = filepath.Join(h.tmpDir, "account-"+strconv.FormatUint(n, 10))
		if err := os.MkdirAll(tmpDir, 0o700); err != nil {
			h.log.Warn("no scratch directory for the account", zap.Error(err))
			tmpDir = ""
		}
	}
	acct, err := openAccount(accountDeps{
		cfg:      cfg,
		startup:  h.startup,
		log:      h.log,
		tmpDir:   tmpDir,
		notifier: h.notifier,
		connect:  h.connect,
		// The session identity is recorded once the login succeeds. Who
		// logged in reaches the clients through the account projection.
		onAuth: func(int64, string) {
			if err := accountstate.Record(h.startup.stateDir, h.startup.sessionFile); err != nil {
				h.log.Error("record account identity", zap.Error(err))
			}
		},
	})
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.acct = acct
	h.mu.Unlock()
	h.clients.Join(acct.owner)
	return acct, nil
}

// listen ends the current account when a client asks: for an end the
// connection did not bring about itself, such as leaving a banned account
// (#297).
func (h *Host) listen(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case reason := <-h.clients.EndRequests():
			go h.endAccount(ctx, h.current(), reason)
		}
	}
}

// launch runs acct, listening for what its connection ends with.
func (h *Host) launch(ctx context.Context, acct *account) {
	acct.run(ctx, h.connectionEnded(ctx, acct))
}

// connectionEnded is told what acct's connection ended with. A log out ends the
// account and starts the next one. Anything else is the account's to show its
// clients, which it does through the account projection while the program
// still runs (#283, #309); a client that exits on it says so in its exit code.
func (h *Host) connectionEnded(ctx context.Context, acct *account) func(error) {
	return func(err error) {
		if ctx.Err() != nil {
			return
		}
		if reason, ok := core.EndReasonOf(err); ok {
			// On a goroutine of its own: ending the account waits for this one,
			// which is still the connection's.
			go h.endAccount(ctx, acct, reason)
			return
		}
		if telerr.Of(err) == telerr.AccountBanned {
			h.tellNobodyWatching(bannedNotice)
		}
	}
}

// What the desktop is told when the account stops delivering with no window
// open to say so (#211).
const (
	loggedOutNotice = "tele was logged out, and no longer delivers messages. Open tele to log in again."
	bannedNotice    = "Telegram has banned the account tele was logged in to, and tele no longer delivers messages."
)

// tellNobodyWatching raises a desktop notification saying body when no window
// is attached to say it instead; a command running is not a person looking
// (#213). It is not a notification about a chat: it goes through no
// notification decision, and the desktop sink's setting does not reach it,
// since it is the one way to learn the service stopped delivering (#211).
func (h *Host) tellNobodyWatching(body string) {
	if h.clients.Windows() > 0 {
		return
	}
	if err := h.notifier.Notify("tele", body); err != nil {
		h.log.Warn("the desktop could not be told the account stopped delivering", zap.Error(err))
	}
}

// endAccount ends acct for reason and starts the next account in its place.
// The clients hear it first, before anything is closed: their calls in flight
// are answered as ended rather than failing on whatever the stop closes under
// them. Then the account is stopped whole, removed from disk with its session,
// and the next one is opened and the clients seated there (#297, #309). If the
// removal fails, the next account is not opened: starting over the last one's
// files is what this exists to prevent.
//
// Only the account running now can end: a second request for one that has
// already ended - the connection and a client both asking - would otherwise
// remove the next account's files.
func (h *Host) endAccount(ctx context.Context, acct *account, reason core.EndReason) {
	h.switching.Lock()
	defer h.switching.Unlock()
	if ctx.Err() != nil {
		return // the process is ending; its stop stops the account
	}
	if h.current() != acct {
		return
	}
	discarded := acct.owner.UnsentCount()
	h.tellNobodyWatching(loggedOutNotice)
	h.clients.Leave(core.AccountEnded{Reason: reason, Discarded: discarded})
	acct.stop()

	if err := accountstate.End(acct.startup.stateDir, acct.startup.sessionFile); err != nil {
		h.log.Error("the account that ended could not be removed", zap.Error(err))
		h.clients.Tell(core.HostFailure{Err: telerr.From(fmt.Errorf("the account that ended could not be removed: %w", err))})
		return
	}
	if acct.tmpDir != "" {
		_ = os.RemoveAll(acct.tmpDir)
	}
	h.log.Info("account ended", zap.String("reason", string(reason)), zap.Int("discarded", discarded))

	next, err := h.openNext()
	if err != nil {
		h.log.Error("the next account could not be opened", zap.Error(err))
		h.clients.Tell(core.HostFailure{Err: telerr.From(err)})
		return
	}
	h.launch(ctx, next)
}
