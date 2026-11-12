// Package protocol names what crosses the wire in a family: the windows a
// client subscribes to and the events it is told. Both sides of a connection
// read these registries, so a name is given in one place and means the same
// thing at either end (#209).
//
// A name says what a value is rather than what its Go type is called, so a
// type can be renamed without changing what goes on the wire. Changing a name
// is changing the protocol.
package protocol

import (
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
)

// Windows names every window a client may subscribe with.
var Windows = windows()

func windows() *wire.Kinds[project.Window] {
	k := wire.NewKinds[project.Window]()
	wire.Register[project.ChatListWindow](k, "chat_list")
	wire.Register[project.TopicListWindow](k, "topic_list")
	wire.Register[project.HistoryWindow](k, "history")
	wire.Register[project.AccountWindow](k, "account")
	return k
}

// Events names every event a client may be told, the owner's and the host's.
var Events = events()

func events() *wire.Kinds[core.Event] {
	k := wire.NewKinds[core.Event]()
	wire.Register[core.Incoming](k, "incoming")
	wire.Register[core.Typing](k, "typing")
	wire.Register[core.Progress](k, "progress")
	wire.Register[core.DownloadProgress](k, "download_progress")
	wire.Register[core.Failure](k, "failure")
	wire.Register[core.Notification](k, "notification")
	wire.Register[core.AccountEnded](k, "account_ended")
	wire.Register[core.MessageGone](k, "message_gone")
	wire.Register[core.HostFailure](k, "host_failure")
	wire.Register[core.HostStopping](k, "host_stopping")
	return k
}
