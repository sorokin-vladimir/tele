package wire

import (
	"encoding/json"
	"time"
)

// The calls about the host rather than about an account. They are the
// protocol's, like hello, and are answered whether or not an owner is seated,
// on the login screen and after a log out alike: the config file is the
// host's, and it outlives the accounts that pass through it (#315).
const (
	// MethodSettings asks for the host's settings as it holds them.
	MethodSettings = "$settings"
	// MethodSet changes one host setting. The host checks it, writes it into
	// its own config file and reloads: a window never writes a host setting
	// itself.
	MethodSet = "$set"
	// MethodReload asks the host to read its config file again.
	MethodReload = "$reload"
	// MethodStatus asks the host how it and its account are doing. It changes
	// nothing, which is why it is asked of any host whose protocol is this
	// one, whatever the two binaries' versions (#208).
	MethodStatus = "$status"
	// MethodRestart asks a service to restart, which its service manager does
	// on the binary installed now. A newer client asks it, and only of a
	// service: any other host is somebody's session (#183).
	MethodRestart = "$restart"
)

// MethodUninstall tells a service it is being uninstalled: the stop its
// service manager makes next is for good, and it says so to its clients
// (#182).
const MethodUninstall = "$uninstall"

// UninstallResult is what the host said to MethodUninstall.
type UninstallResult struct {
	// Refused says why the host is not one to uninstall. Empty when it took
	// note.
	Refused string `json:"refused,omitempty"`
}

// RestartResult is what the host said to MethodRestart. A refusal is an
// answer, not a failure of the call.
type RestartResult struct {
	// Refused says why the host will not restart. Empty when it is restarting:
	// it stops after answering.
	Refused string `json:"refused,omitempty"`
}

// HostStatus is what the host answers MethodStatus with.
type HostStatus struct {
	// Since is when the host started.
	Since time.Time `json:"since"`
	// Windows and Commands are how many of each are attached, the command
	// asking left out (#213).
	Windows  int `json:"windows"`
	Commands int `json:"commands"`
	// AccountCommands is cli.account_commands: what the host lets commands do
	// with the account (#181).
	AccountCommands CommandAccess `json:"account_commands"`
	// Account is how the account is doing, nil between one account and the
	// next.
	Account *AccountStatus `json:"account,omitempty"`
}

// CommandAccess is what the host lets commands do with the account, as
// cli.account_commands says: everything, read it and change nothing, or
// nothing at all. A window is never held to it (#181).
type CommandAccess string

const (
	CommandsAll  CommandAccess = "all"
	CommandsRead CommandAccess = "read"
	CommandsNone CommandAccess = "none"
)

// AccountStatus is how the host's account is doing.
type AccountStatus struct {
	Phase Phase `json:"phase"`
	// User is the logged-in user's username, empty until Telegram has said
	// or when they have none.
	User string `json:"user,omitempty"`
	// Ended is what the connection ended with, in words, empty while it runs.
	Ended string `json:"ended,omitempty"`
	// Link is whether the link to Telegram is up, and LinkSince since when.
	Link      Link      `json:"link"`
	LinkSince time.Time `json:"link_since,omitzero"`
	// LastUpdate is when the last update from Telegram arrived, zero when none
	// has.
	LastUpdate time.Time `json:"last_update,omitzero"`
	// Outbox is how many sends have not gone out yet.
	Outbox int `json:"outbox"`
	// Throttle is the flood wait somebody's call ran into, nil when there is
	// none or it is over (#212).
	Throttle *Throttle `json:"throttle,omitempty"`
}

// Throttle is a method Telegram asked to wait for, and until when.
type Throttle struct {
	Method string    `json:"method"`
	Until  time.Time `json:"until"`
}

// Phase is where the account stands.
type Phase string

const (
	PhaseConnecting Phase = "connecting"
	PhaseLoggingIn  Phase = "logging_in"
	PhaseLoading    Phase = "loading"
	PhaseReady      Phase = "ready"
)

// Link is whether the link to Telegram is up.
type Link string

const (
	// LinkUnknown: the link has not come up yet.
	LinkUnknown Link = "unknown"
	LinkUp      Link = "up"
	LinkDown    Link = "down"
)

// Warning is something wrong with one of the host's settings, as a client
// shows it. One with an ID is shown only until it has been seen.
type Warning struct {
	Text string `json:"text"`
	ID   string `json:"id,omitempty"`
}

// HostSettings is what the host answers MethodSettings with.
type HostSettings struct {
	// Config is the file the host reads its settings from.
	Config   string    `json:"config"`
	Settings []Setting `json:"settings"`
}

// Setting is one host setting as the host holds it.
type Setting struct {
	Key string `json:"key"`
	// Value is what the setting is worth, encoded as JSON. Which Go type it
	// decodes into is the setting's, which the client knows from the key.
	Value json.RawMessage `json:"value,omitempty"`
	// Described is said in place of a value the host does not show, such as
	// which built-in app key is in force when the file names none.
	Described string `json:"described,omitempty"`
	// Default says the file does not name the setting.
	Default bool `json:"default,omitempty"`
}

// SetParams is what MethodSet carries.
type SetParams struct {
	Key string `json:"key"`
	// Value is the new value encoded as JSON. Nil resets the setting to its
	// default: absence is what a default is.
	Value json.RawMessage `json:"value,omitempty"`
}

// SetResult is what the host said of a change. A refusal is an answer, not a
// failure of the call: the host understood and would not write it.
type SetResult struct {
	// Refused says why the value was not written, in words for the row it is
	// about. Empty when it was.
	Refused  string    `json:"refused,omitempty"`
	Warnings []Warning `json:"warnings,omitempty"`
}

// ReloadResult is what the host found when it read its file again.
type ReloadResult struct {
	// Failed says why the file was not taken, in which case the host goes on
	// with the settings it had. Empty when it was.
	Failed   string    `json:"failed,omitempty"`
	Warnings []Warning `json:"warnings,omitempty"`
}
