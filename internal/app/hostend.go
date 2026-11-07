package app

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/ipc/serve"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/settings"
)

// HostEnd is the host's own end of every connection: what it says of itself in
// hello, and its answers to the calls about its settings. It belongs to the
// process rather than to an account, like the config file it reads, so it
// answers before the first owner is open and between owners alike (#315).
//
// A host setting is read, checked and written here and nowhere else: a window
// that changes one asks, and the host writes it into its own file and applies
// it.
type HostEnd struct {
	store *config.Store
	hello wire.Hello

	mu sync.Mutex
	// apply hands a config the host has just read to whatever runs on it.
	// Nil until the host has an owner to hand it to: one opened later starts
	// from the store as it is.
	apply func(*config.Config)
	// since is when the host started; windows, commands and account are what
	// it counts and tells of when asked how it is doing. Nil until there is
	// something to count.
	since    time.Time
	windows  func() int
	commands func() int
	account  func() *wire.AccountStatus
	// restart stops the host to be started again, nil while nothing would.
	restart func()
	// uninstalled is set once the host, a service, was told it is being
	// uninstalled.
	uninstalled bool
}

var _ serve.Host = (*HostEnd)(nil)

// NewHostEnd is the host's end over the config file in store, saying hello as
// hello says. The host starts now.
func NewHostEnd(store *config.Store, hello wire.Hello) *HostEnd {
	return &HostEnd{store: store, hello: hello, since: time.Now()}
}

// StatusFrom sets what the host counts its windows and its commands with, and
// where it learns how its account is doing.
func (e *HostEnd) StatusFrom(windows, commands func() int, account func() *wire.AccountStatus) {
	e.mu.Lock()
	e.windows, e.commands, e.account = windows, commands, account
	e.mu.Unlock()
}

// RestartWith sets what stops the host to be started again. It must not block.
func (e *HostEnd) RestartWith(restart func()) {
	e.mu.Lock()
	e.restart = restart
	e.mu.Unlock()
}

// Restart restarts the host when it is a service, which its service manager
// starts again on the binary installed now. Any other host is somebody's
// session, and refuses (#183). The host stops once the client asking has its
// answer.
func (e *HostEnd) Restart() (wire.RestartResult, func()) {
	if e.hello.Kept != wire.KeptByService {
		return wire.RestartResult{Refused: "this tele is not a service, and only its own window or its own terminal stops it"}, nil
	}
	e.mu.Lock()
	restart := e.restart
	e.mu.Unlock()
	if restart == nil {
		return wire.RestartResult{Refused: "this tele is still starting"}, nil
	}
	return wire.RestartResult{}, restart
}

// AccountCommands is cli.account_commands as the host read it last: what a
// command may do with the account (#181).
func (e *HostEnd) AccountCommands() wire.CommandAccess {
	return wire.CommandAccess(e.store.Current().CLI.AccountCommands)
}

// Uninstall takes note that the host, a service, is being uninstalled: the
// stop its service manager makes next is for good. Any other host is not one
// to uninstall, and refuses (#182).
func (e *HostEnd) Uninstall() wire.UninstallResult {
	if e.hello.Kept != wire.KeptByService {
		return wire.UninstallResult{Refused: "this tele is not a service"}
	}
	e.mu.Lock()
	e.uninstalled = true
	e.mu.Unlock()
	return wire.UninstallResult{}
}

// StopReason is why the host stops when it is told to quit: for good, once it
// was told it is being uninstalled.
func (e *HostEnd) StopReason() core.StopReason {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.uninstalled {
		return core.StopUninstalled
	}
	return core.StopQuit
}

// Status is how the host is doing. The command asking is one of the commands
// counted, and is left out: it asks about the others (#208, #213).
func (e *HostEnd) Status() wire.HostStatus {
	e.mu.Lock()
	windows, commands, account := e.windows, e.commands, e.account
	e.mu.Unlock()
	s := wire.HostStatus{Since: e.since, AccountCommands: e.AccountCommands()}
	if windows != nil {
		s.Windows = windows()
	}
	if commands != nil {
		s.Commands = max(commands()-1, 0)
	}
	if account != nil {
		s.Account = account()
	}
	return s
}

// ApplyTo sets what a config the host has just read is handed to.
func (e *HostEnd) ApplyTo(apply func(*config.Config)) {
	e.mu.Lock()
	e.apply = apply
	e.mu.Unlock()
}

// Hello is what the host says of itself, with what is wrong with its settings
// as it runs on them now.
func (e *HostEnd) Hello() wire.Hello {
	h := e.hello
	h.Warnings = hostWarnings(e.store.Current())
	return h
}

// Settings answers with every host setting as the host holds it.
func (e *HostEnd) Settings() wire.HostSettings {
	out := wire.HostSettings{Config: e.store.Path()}
	for _, entry := range e.store.Entries() {
		if entry.Process != settings.Host {
			continue
		}
		s := wire.Setting{Key: entry.Key, Default: e.store.IsDefault(entry.Key)}
		v, _ := e.store.Value(entry.Key)
		if described, ok := v.(settings.Described); ok {
			s.Described = string(described)
		} else if raw, err := json.Marshal(v); err == nil {
			s.Value = raw
		}
		out.Settings = append(out.Settings, s)
	}
	return out
}

// SetSetting writes one host setting into the host's file and applies what
// the file then says. Anything it will not write is refused in words for the
// row the change was made on.
func (e *HostEnd) SetSetting(p wire.SetParams) wire.SetResult {
	entry, ok := config.Setting(p.Key)
	if !ok {
		return wire.SetResult{Refused: p.Key + " is not a setting"}
	}
	if entry.Process != settings.Host {
		return wire.SetResult{Refused: "a setting of each window, not of the host"}
	}
	v, err := config.DecodeValue(p.Key, p.Value)
	if err != nil {
		return wire.SetResult{Refused: refusal(err, p.Key)}
	}
	if err := e.store.Set(p.Key, v); err != nil {
		return wire.SetResult{Refused: refusal(err, p.Key)}
	}
	cfg := e.store.Current()
	e.applyNow(cfg)
	return wire.SetResult{Warnings: hostWarnings(cfg)}
}

// Reload reads the host's file again and applies it. A file that no longer
// loads leaves the host on what it had.
func (e *HostEnd) Reload() wire.ReloadResult {
	if err := e.store.Reload(); err != nil {
		return wire.ReloadResult{Failed: err.Error()}
	}
	cfg := e.store.Current()
	e.applyNow(cfg)
	return wire.ReloadResult{Warnings: hostWarnings(cfg)}
}

func (e *HostEnd) applyNow(cfg *config.Config) {
	e.mu.Lock()
	apply := e.apply
	e.mu.Unlock()
	if apply != nil {
		apply(cfg)
	}
}

// refusal drops the key from the front of err: the row it is shown on already
// says which setting it is about.
func refusal(err error, key string) string {
	return strings.TrimPrefix(err.Error(), key+": ")
}

// hostWarnings is what is wrong with the host's own settings in cfg.
func hostWarnings(cfg *config.Config) []wire.Warning {
	var out []wire.Warning
	for _, w := range cfg.Warnings {
		if w.Process == settings.Host {
			out = append(out, wire.Warning{Text: w.Text, ID: w.ID})
		}
	}
	return out
}
