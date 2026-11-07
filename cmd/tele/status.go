package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sorokin-vladimir/tele/internal/ipc"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/version"
)

// newStatusCmd is `tele daemon status`: how the tele hosting the account is
// doing, asked over the endpoint of any host, a window's included (#208).
func newStatusCmd(o *options) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Say how the tele hosting the account is doing",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return newInstaller(cmd).status(cmd.Context(), *o, asJSON, cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print it as JSON, as docs/cli.md describes")
	return cmd
}

// noHostCode is what status exits with when no host answers: a script tells
// it from any other failure.
const noHostCode = 3

// status asks the host of the state directory how it is doing, and says what
// it said, and what is wrong with the service installed for the config, if
// one is: whether or not a host answers, since a service whose command is gone
// is exactly one that does not (#182).
//
// As JSON, a host that does not answer leaves nothing to print: the exit code
// says it, and the warning goes to errOut, so that what a script reads is
// either the whole answer or nothing (#213).
func (in installer) status(ctx context.Context, o options, asJSON bool, errOut io.Writer) error {
	s, err := newSetup(o.cfgPath)
	if err != nil {
		return err
	}
	a, err := askHost(ctx, s)
	warning := in.installedWarning(s)
	if err != nil {
		if warning != "" {
			w := in.out
			if asJSON {
				w = errOut
			}
			_, _ = fmt.Fprintln(w, warning)
		}
		return err
	}
	if asJSON {
		raw, err := statusJSON(a.stateDir, a.host, a.status, a.mismatch, a.mine, warning)
		if err != nil {
			return err
		}
		_, err = in.out.Write(raw)
		return err
	}
	if _, err := fmt.Fprint(in.out, statusText(a.stateDir, a.host, a.status, a.mismatch, a.mine, time.Now())); err != nil {
		return err
	}
	if warning != "" {
		_, _ = fmt.Fprintln(in.out, warning)
	}
	return nil
}

// asked is what the host of stateDir said when asked how it is doing: what it
// said of itself, how it is doing, and, set in mismatch when it speaks another
// protocol, why it was asked nothing more. mine is what this tele said.
type asked struct {
	stateDir string
	host     wire.Hello
	status   wire.HostStatus
	mismatch error
	mine     wire.Hello
}

// installedWarning is what is wrong with the service installed for the config
// read by s, empty when nothing is, or nothing is installed.
func (in installer) installedWarning(s setup) string {
	m, err := in.manager()
	if err != nil {
		return ""
	}
	u, err := unitOf(s, "")
	if err != nil {
		return ""
	}
	binary, ok, err := m.Installed(u.Name)
	if err != nil {
		return fmt.Sprintf("The service %s cannot be read: %v", u.Name, err)
	}
	if !ok {
		return ""
	}
	return unitWarning(binary, in.versionOf, version.Version)
}

// unitWarning is what is wrong with a service that runs binary, for a tele of
// version mine to say: a command that is gone, which the service manager can
// no longer start, or another tele than this one, which a restart would start.
func unitWarning(binary string, versionOf func(string) (string, error), mine string) string {
	v, err := versionOf(binary)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Sprintf("The service runs %s, which is not there any more.\n`tele daemon install` registers it again.", binary)
	case err != nil:
		return fmt.Sprintf("The service runs %s, which did not say its version: %v", binary, err)
	case v != mine:
		return fmt.Sprintf("The service runs %s, which is tele %s, not this tele (%s).\n`tele daemon install` from this one registers it again.", binary, v, mine)
	default:
		return ""
	}
}

// askHost asks the host of the state directory read by s how it is doing.
func askHost(ctx context.Context, s setup) (asked, error) {
	stateDir := s.store.Current().StateDir
	none := &exitCode{code: noHostCode, msg: fmt.Sprintf("No tele hosts %s.", stateDir)}
	// The endpoint is named after the state directory's resolved path: with no
	// directory, nothing has ever hosted it.
	if _, err := os.Stat(stateDir); errors.Is(err, os.ErrNotExist) {
		return asked{}, none
	}
	ep, err := ipc.EndpointFor(appName, stateDir)
	if err != nil {
		return asked{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	conn, err := ipc.Dial(ctx, ep)
	if errors.Is(err, ipc.ErrNoHost) {
		return asked{}, none
	}
	if err != nil {
		return asked{}, fmt.Errorf("reaching the tele that hosts %s: %w", stateDir, err)
	}
	mine := wire.Hello{Protocol: wire.ProtocolVersion, Binary: version.Version, Config: s.cfgPath, Kind: wire.KindCommand}
	host, status, err := dial.AskStatus(ctx, conn, mine)
	var mismatch *dial.Mismatch
	if err != nil && !errors.As(err, &mismatch) {
		return asked{}, fmt.Errorf("asking the tele that hosts %s: %w", stateDir, err)
	}
	return asked{stateDir: stateDir, host: host, status: status, mismatch: err, mine: mine}, nil
}

// exitCode ends tele with code, after saying msg.
type exitCode struct {
	code int
	msg  string
}

func (e *exitCode) Error() string { return e.msg }

// statusText is what `tele daemon status` prints of the host of stateDir: what
// it said of itself in host, how it is doing in status, and how this tele,
// saying mine, stands to it. mismatch is set when the host speaks another
// protocol, and status was then never asked (#208).
func statusText(stateDir string, host wire.Hello, status wire.HostStatus, mismatch error, mine wire.Hello, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "tele %s (pid %d) hosts %s, %s", host.Binary, host.PID, stateDir, keptBy(host.Kept))
	if mismatch == nil {
		fmt.Fprintf(&b, ", up %s", since(status.Since, now))
	}
	b.WriteString(".\n")
	if line := versions(mine, host); line != "" {
		b.WriteString(line + "\n")
	}
	if mismatch != nil {
		return b.String()
	}

	fmt.Fprintf(&b, "Windows: %d, commands: %d\n", status.Windows, status.Commands)
	switch status.AccountCommands {
	case wire.CommandsRead:
		b.WriteString("Commands may read the account and change nothing: cli.account_commands is read.\n")
	case wire.CommandsNone:
		b.WriteString("Commands may not act on the account: cli.account_commands is none.\n")
	}
	a := status.Account
	if a == nil {
		b.WriteString("Account: none, the next one is opening\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Account: %s\n", account(a))
	fmt.Fprintf(&b, "Telegram: %s\n", link(a, now))
	if a.LastUpdate.IsZero() {
		b.WriteString("Last update: none\n")
	} else {
		fmt.Fprintf(&b, "Last update: %s ago\n", since(a.LastUpdate, now))
	}
	fmt.Fprintf(&b, "Outbox: %d waiting\n", a.Outbox)
	if t := a.Throttle; t != nil && t.Until.After(now) {
		fmt.Fprintf(&b, "Telegram asks to wait: %s, on %s\n", t.Until.Sub(now).Round(time.Second), t.Method)
	}
	return b.String()
}

func keptBy(k wire.Keeper) string {
	switch k {
	case wire.KeptByService:
		return "kept running as a service"
	case wire.KeptByDaemon:
		return "kept running by tele daemon"
	default:
		return "kept running by its window"
	}
}

// versions says how this tele stands to the host, empty when they are the
// same build.
func versions(mine, host wire.Hello) string {
	if mine.Binary == host.Binary && mine.Protocol == host.Protocol {
		return ""
	}
	this := "This tele is " + mine.Binary
	switch wire.Decide(mine, host) {
	case wire.RestartHost:
		return this + ", newer: its next window restarts the service on it."
	case wire.RestartClient:
		return this + ", older than the service: update it."
	case wire.Refuse:
		return this + " and cannot attach to it: the two speak different protocols."
	default:
		return this + "."
	}
}

func account(a *wire.AccountStatus) string {
	if a.Ended != "" {
		return "the connection ended: " + a.Ended
	}
	s := map[wire.Phase]string{
		wire.PhaseConnecting: "connecting",
		wire.PhaseLoggingIn:  "logging in",
		wire.PhaseLoading:    "loading",
		wire.PhaseReady:      "ready",
	}[a.Phase]
	if a.User != "" {
		s += ", logged in as @" + a.User
	}
	return s
}

func link(a *wire.AccountStatus, now time.Time) string {
	switch a.Link {
	case wire.LinkUp:
		return fmt.Sprintf("up, connected %s ago", since(a.LinkSince, now))
	case wire.LinkDown:
		return fmt.Sprintf("down, dropped %s ago", since(a.LinkSince, now))
	default:
		return "not connected yet"
	}
}

// since is how long ago t was, to the second.
func since(t, now time.Time) string {
	return now.Sub(t).Round(time.Second).String()
}
