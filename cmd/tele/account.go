package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/version"
)

// accountSurface is the owner's surface as a command holds it: what it may ask, and
// the stream it is told the account's state on (#181).
type accountSurface interface {
	core.Surface
	Stream() <-chan any
}

// readyTimeout bounds how long a command waits for the account to be ready to
// be asked, and then for its answer.
const readyTimeout = 30 * time.Second

// reachAccount attaches to the tele holding the state directory read by s, as
// a command: one that asks and leaves. It attaches the way a window does, so
// a service older than this tele is restarted on the new one, and a service
// that goes is waited for. With no tele there it exits 3: a command never
// starts one of its own (#181).
func reachAccount(ctx context.Context, s setup) (*dial.Attachment, error) {
	stateDir := s.store.Current().StateDir
	none := &exitCode{code: noHostCode, msg: fmt.Sprintf(
		"No tele holds %s.\nOpen tele, or install the service with `tele daemon install`.", stateDir)}
	// The endpoint is named after the state directory's resolved path: with no
	// directory, nothing has ever hosted it.
	if _, err := os.Stat(stateDir); errors.Is(err, os.ErrNotExist) {
		return nil, none
	}
	ep, err := ipc.EndpointFor(appName, stateDir)
	if err != nil {
		return nil, err
	}
	conn, err := ipc.Dial(ctx, ep)
	if errors.Is(err, ipc.ErrNoHost) {
		return nil, none
	}
	if err != nil {
		return nil, errors.New(reachFailure(err, stateDir))
	}
	mine := wire.Hello{Protocol: wire.ProtocolVersion, Binary: version.Version, Config: s.cfgPath, Kind: wire.KindCommand}
	att, err := hosting.Attach(ctx, hosting.Reached{Endpoint: ep, Conn: conn}, mine)
	if err != nil {
		return nil, errors.New(reachFailure(err, stateDir))
	}
	return att, nil
}

// heldAccount is the account a command asks, once it is ready: its surface,
// who it is logged in as, and the command's own config, since a client's
// settings are read by the client (#181).
type heldAccount struct {
	accountSurface
	me  project.LoggedInUser
	cfg *config.Config
}

// askAccount runs ask against the account held for the config o names, once
// it is ready. Reaching it and waiting for it share readyTimeout; ask has
// within of its own, since a send waits on top of that.
func askAccount(ctx context.Context, o options, within time.Duration, ask func(context.Context, heldAccount) error) error {
	s, err := newSetup(o.cfgPath)
	if err != nil {
		return err
	}
	readyCtx, cancelReady := context.WithTimeout(ctx, readyTimeout)
	defer cancelReady()
	att, err := reachAccount(readyCtx, s)
	if err != nil {
		return err
	}
	defer func() { _ = att.Close() }()
	acct, err := awaitReady(readyCtx, att)
	if err != nil {
		return err
	}
	askCtx, cancelAsk := context.WithTimeout(ctx, within)
	defer cancelAsk()
	return commandsOff(ask(askCtx, heldAccount{accountSurface: att, me: acct.LoggedInUser, cfg: s.store.Current()}))
}

// firstDelta subscribes to w and returns what the subscription is answered
// with first: the window whole, as every subscription begins (#181).
func firstDelta(ctx context.Context, acc accountSurface, w project.Window) (project.Delta, error) {
	sub := acc.Subscribe(w)
	defer acc.Unsubscribe(sub)
	for {
		select {
		case <-ctx.Done():
			return project.Delta{}, fmt.Errorf("the account did not answer in time: %w", ctx.Err())
		case msg, ok := <-acc.Stream():
			if !ok {
				return project.Delta{}, &exitCode{code: noHostCode, msg: "The tele that holds the account went away."}
			}
			if failure, ok := msg.(core.HostFailure); ok && failure.Err != nil && failure.Err.Kind == telerr.CommandsOff {
				return project.Delta{}, commandsOff(failure.Err)
			}
			if _, ended := msg.(core.AccountEnded); ended {
				return project.Delta{}, errors.New("the account ended before it answered")
			}
			if d, ok := msg.(project.Delta); ok && d.Sub == sub {
				return d, nil
			}
		}
	}
}

// notLoggedInCode is what a command exits with when the account is not logged
// in: logging in takes a person, and a command does not wait for one.
const notLoggedInCode = 4

// notFoundCode is what a command exits with when the chat, topic or folder it
// was given is not there.
const notFoundCode = 5

// commandsOffCode is what a command exits with when the tele holding the
// account does not let commands do what it asked: anything, or anything but
// read (#181).
const commandsOffCode = 7

// commandsOff turns the refusal of a tele that does not let commands reach
// the account into exit 7, naming the setting; any other error is left as it
// is.
func commandsOff(err error) error {
	if telerr.Of(err) != telerr.CommandsOff {
		return err
	}
	return &exitCode{code: commandsOffCode, msg: "The tele holding the account does not let commands do this: " +
		"cli.account_commands in its config says what they may do."}
}

// awaitReady returns once the account is ready to be asked: its chat list is
// loaded. Connecting and loading pass by themselves and are waited for; a
// login waits on a person, and ends the command.
func awaitReady(ctx context.Context, acc accountSurface) (project.AccountContents, error) {
	sub := acc.Subscribe(project.AccountWindow{})
	for {
		select {
		case <-ctx.Done():
			return project.AccountContents{}, fmt.Errorf("the account was not ready in time: %w", ctx.Err())
		case msg, ok := <-acc.Stream():
			if !ok {
				return project.AccountContents{}, &exitCode{code: noHostCode, msg: "The tele that holds the account went away."}
			}
			if failure, ok := msg.(core.HostFailure); ok && failure.Err != nil && failure.Err.Kind == telerr.CommandsOff {
				return project.AccountContents{}, commandsOff(failure.Err)
			}
			if _, ended := msg.(core.AccountEnded); ended {
				// The subscription ended with its owner; the next owner is
				// given this one.
				sub = acc.Subscribe(project.AccountWindow{})
				continue
			}
			d, ok := msg.(project.Delta)
			if !ok || d.Sub != sub || d.Account == nil {
				continue
			}
			if ended := d.Account.Contents.Ended; ended != nil {
				if _, logOut := core.EndReasonOf(ended); logOut {
					// The account is over; AccountEnded follows, and the
					// next one after it.
					continue
				}
				return project.AccountContents{}, fmt.Errorf("the account's connection ended: %w", ended)
			}
			switch d.Account.Contents.Phase {
			case project.AccountReady:
				return d.Account.Contents, nil
			case project.AccountLoggingIn:
				return project.AccountContents{}, &exitCode{code: notLoggedInCode, msg: "The account is not logged in. Run tele to log in."}
			}
		}
	}
}
