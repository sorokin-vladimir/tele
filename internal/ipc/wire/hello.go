package wire

import (
	"golang.org/x/mod/semver"
)

// ProtocolVersion is the shape of what this binary says on the wire. It goes up
// with any change the other side could misread. Two binaries with the same one
// understand each other, whatever their own versions.
const ProtocolVersion = 6

// MethodHello is the first thing a client says, and the host's answer is its
// own. Nothing else is said before it.
const MethodHello = "$hello"

// Hello is what each side says of itself when a connection opens.
type Hello struct {
	// Protocol is the side's ProtocolVersion.
	Protocol int `json:"protocol"`
	// Binary is the version of the binary, "dev" for one built locally.
	Binary string `json:"binary"`
	// Config is the config file the side reads.
	Config string `json:"config"`
	// Warnings is what was wrong with the host's settings when it started,
	// said by a host only, so the first window to attach shows it.
	Warnings []Warning `json:"warnings,omitempty"`
	// Kept says what keeps a host running, said by a host only.
	Kept Keeper `json:"kept,omitempty"`
	// PID is the host's process, said by a host only, so whoever finds it in
	// the way can name it.
	PID int `json:"pid,omitempty"`
	// Kind is what kind of client this is, said by a client only (#213).
	Kind ClientKind `json:"kind,omitempty"`
}

// ClientKind is what kind of client is attached. Only a window is someone
// looking: it alone focuses a history, draws a notification and counts as
// someone watching, while a command asks once and leaves.
type ClientKind string

const (
	// KindWindow: a client a person looks at and drives.
	KindWindow ClientKind = "window"
	// KindCommand: a client that asks once, prints the answer and ends.
	KindCommand ClientKind = "command"
)

// Keeper is what keeps a host running. Only a service is started again when
// it exits, which is why a newer client may ask only a service to restart: any
// other host is somebody's session and is never restarted from outside.
type Keeper string

const (
	// KeptByWindow: a tele with a window of its own hosts, for as long as it
	// is open.
	KeptByWindow Keeper = "window"
	// KeptByDaemon: `tele daemon` run by hand hosts with no window, for as
	// long as whoever started it lets it run.
	KeptByDaemon Keeper = "daemon"
	// KeptByService: the operating system's service manager keeps the host
	// running, and starts it again whenever it exits.
	KeptByService Keeper = "service"
)

// Verdict is what a client does about the host it reached.
type Verdict int

const (
	// Proceed: the two understand each other.
	Proceed Verdict = iota + 1
	// RestartHost: the host is a service running an older binary. The client
	// asks it to restart, and the service manager starts it on the new one.
	RestartHost
	// RestartClient: the host is a service running a newer binary than this
	// client. The client says tele was updated and stops; it never asks a
	// newer host to go back.
	RestartClient
	// Refuse: the two do not understand each other, and the host is someone's
	// session, which is not to be ended from outside.
	Refuse
)

// Decide says what a client with client's hello does about a host that
// answered with host's. A service follows its newer side, whichever it is: an
// upgrade restarts the service lazily, on the first newer client, and an old
// client left open across the upgrade must not take the service back. A host a
// terminal keeps running is used while the two understand each other and
// refused once they do not. Versions that cannot be ordered - a local build -
// leave only the protocol to decide.
func Decide(client, host Hello) Verdict {
	order, ordered := compareBinaries(client.Binary, host.Binary)
	if host.Kept == KeptByService && ordered && order != 0 {
		if order > 0 {
			return RestartHost
		}
		return RestartClient
	}
	if client.Protocol == host.Protocol {
		return Proceed
	}
	return Refuse
}

// compareBinaries orders two release versions, such as "2.0.0-beta.1" and
// "2.0.0". It reports false when either is not one.
func compareBinaries(a, b string) (int, bool) {
	va, vb := "v"+a, "v"+b
	if !semver.IsValid(va) || !semver.IsValid(vb) {
		return 0, false
	}
	return semver.Compare(va, vb), true
}
