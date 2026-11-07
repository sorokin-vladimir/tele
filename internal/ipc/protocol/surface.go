package protocol

import "github.com/sorokin-vladimir/tele/internal/core"

// surface is every method of core.Surface, each named and taken as a method
// expression: a function whose first argument is the surface it is called on.
//
// It is written out rather than read off the interface by reflection on
// purpose. A method looked up by reflection with an index or a name the linker
// cannot see - reflect.Type.Method, reflect.Value.Method, MethodByName - makes
// the linker keep every exported method of every type in the binary, and with
// the thousands of generated Telegram types that doubles tele's size (#208).
// A test holds this list to the interface, so a method added there and not
// here fails it.
var surface = map[string]any{
	"Subscribe":        core.Surface.Subscribe,
	"MoveWindow":       core.Surface.MoveWindow,
	"Unsubscribe":      core.Surface.Unsubscribe,
	"LogOutPreview":    core.Surface.LogOutPreview,
	"LogOut":           core.Surface.LogOut,
	"EndAccount":       core.Surface.EndAccount,
	"AnswerLogin":      core.Surface.AnswerLogin,
	"SwitchLogin":      core.Surface.SwitchLogin,
	"SetMuted":         core.Surface.SetMuted,
	"SetArchived":      core.Surface.SetArchived,
	"SetUnreadMark":    core.Surface.SetUnreadMark,
	"AddToFolder":      core.Surface.AddToFolder,
	"MarkRead":         core.Surface.MarkRead,
	"SetFocus":         core.Surface.SetFocus,
	"ReadReactions":    core.Surface.ReadReactions,
	"ReadMentions":     core.Surface.ReadMentions,
	"EditMessage":      core.Surface.EditMessage,
	"DeleteMessages":   core.Surface.DeleteMessages,
	"SendReaction":     core.Surface.SendReaction,
	"Forward":          core.Surface.Forward,
	"SetTyping":        core.Surface.SetTyping,
	"SaveDraft":        core.Surface.SaveDraft,
	"Send":             core.Surface.Send,
	"SendMedia":        core.Surface.SendMedia,
	"RetryOutbox":      core.Surface.RetryOutbox,
	"DiscardOutbox":    core.Surface.DiscardOutbox,
	"AwaitSend":        core.Surface.AwaitSend,
	"SearchContacts":   core.Surface.SearchContacts,
	"Chats":            core.Surface.Chats,
	"Topics":           core.Surface.Topics,
	"Chat":             core.Surface.Chat,
	"ResolveChat":      core.Surface.ResolveChat,
	"History":          core.Surface.History,
	"FolderFilters":    core.Surface.FolderFilters,
	"GetParticipants":  core.Surface.GetParticipants,
	"KnownUser":        core.Surface.KnownUser,
	"GetUser":          core.Surface.GetUser,
	"FetchMedia":       core.Surface.FetchMedia,
	"SaveMedia":        core.Surface.SaveMedia,
	"InvalidateMedia":  core.Surface.InvalidateMedia,
	"FetchAvatar":      core.Surface.FetchAvatar,
	"InvalidateAvatar": core.Surface.InvalidateAvatar,
}

// Reads are the calls that change nothing about the account: what it holds,
// what Telegram says of it, and the caches it is drawn from. Writes are the
// rest, the ones that change something - in Telegram, in the queue, or in
// what the owner thinks somebody is looking at. Every call is one or the
// other; a test holds them to Calls, so a call added later is placed on
// purpose (#181).
var (
	Reads = set(
		"Subscribe", "MoveWindow", "Unsubscribe", "LogOutPreview",
		"SearchContacts", "Chats", "Topics", "Chat", "ResolveChat", "History",
		"FolderFilters", "GetParticipants", "KnownUser", "GetUser",
		"FetchMedia", "SaveMedia", "InvalidateMedia", "FetchAvatar", "InvalidateAvatar",
		"AwaitSend",
	)
	Writes = set(
		"LogOut", "EndAccount", "AnswerLogin", "SwitchLogin",
		"SetMuted", "SetArchived", "SetUnreadMark", "AddToFolder",
		"MarkRead", "SetFocus", "ReadReactions", "ReadMentions",
		"EditMessage", "DeleteMessages", "SendReaction", "Forward", "SetTyping", "SaveDraft",
		"Send", "SendMedia", "RetryOutbox", "DiscardOutbox",
	)
)

func set(names ...string) map[string]struct{} {
	s := make(map[string]struct{}, len(names))
	for _, n := range names {
		s[n] = struct{}{}
	}
	return s
}
