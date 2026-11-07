package main

import (
	"encoding/json"
	"time"

	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
)

// The shape `tele daemon status --json` prints, documented in docs/cli.md and
// kept within a major version (#213). It is its own rather than the wire's, so
// that what the host and a client say to each other may change between
// betas without a script noticing.
type (
	statusDoc struct {
		StateDir string     `json:"state_dir"`
		Host     helloDoc   `json:"host"`
		Since    *time.Time `json:"since,omitempty"`
		Windows  *int       `json:"windows,omitempty"`
		Commands *int       `json:"commands,omitempty"`
		// AccountCommands is all, read or none; left out for a host never
		// asked.
		AccountCommands string      `json:"account_commands,omitempty"`
		Account         *accountDoc `json:"account,omitempty"`
		This            thisDoc     `json:"this"`
		ServiceWarning  string      `json:"service_warning,omitempty"`
	}
	helloDoc struct {
		Binary   string `json:"binary"`
		Protocol int    `json:"protocol"`
		PID      int    `json:"pid,omitempty"`
		Kept     string `json:"kept"`
		Config   string `json:"config,omitempty"`
	}
	thisDoc struct {
		Binary   string `json:"binary"`
		Protocol int    `json:"protocol"`
	}
	accountDoc struct {
		Phase      string       `json:"phase"`
		User       string       `json:"user,omitempty"`
		Ended      string       `json:"ended,omitempty"`
		Link       string       `json:"link"`
		LinkSince  *time.Time   `json:"link_since,omitempty"`
		LastUpdate *time.Time   `json:"last_update,omitempty"`
		Outbox     int          `json:"outbox"`
		Throttle   *throttleDoc `json:"throttle,omitempty"`
	}
	throttleDoc struct {
		Method string    `json:"method"`
		Until  time.Time `json:"until"`
	}
)

// statusJSON is what `tele daemon status --json` prints of the host of
// stateDir, from what statusText is made of. With mismatch set the host was
// never asked how it is doing, so only what it said in hello is there.
// warning is what is wrong with the service installed, empty when nothing is.
func statusJSON(stateDir string, host wire.Hello, status wire.HostStatus, mismatch error, mine wire.Hello, warning string) ([]byte, error) {
	doc := statusDoc{
		StateDir: stateDir,
		Host: helloDoc{
			Binary: host.Binary, Protocol: host.Protocol, PID: host.PID, Kept: string(host.Kept), Config: host.Config,
		},
		This:           thisDoc{Binary: mine.Binary, Protocol: mine.Protocol},
		ServiceWarning: warning,
	}
	if mismatch == nil {
		doc.Since, doc.Windows, doc.Commands = &status.Since, &status.Windows, &status.Commands
		doc.AccountCommands = string(status.AccountCommands)
		if a := status.Account; a != nil {
			acct := accountDoc{
				Phase: string(a.Phase), User: a.User, Ended: a.Ended, Link: string(a.Link),
				LinkSince: timeOrNil(a.LinkSince), LastUpdate: timeOrNil(a.LastUpdate), Outbox: a.Outbox,
			}
			if t := a.Throttle; t != nil {
				acct.Throttle = &throttleDoc{Method: t.Method, Until: t.Until}
			}
			doc.Account = &acct
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// timeOrNil is t, or nil when it is zero: a time never set is left out.
func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
