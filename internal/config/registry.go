package config

import (
	"fmt"
	"slices"

	"github.com/spf13/viper"

	"github.com/sorokin-vladimir/tele/internal/settings"
)

// registry declares every setting kept in the config file, in the order a
// person meets them - the order of the file itself, so a row on screen and a
// key in the file are found the same way.
//
// This list is written by hand rather than derived from Config. Reflection over
// the struct yields the key path, the Go type and the nesting, and none of the
// rest: that toasts.error_zone accepts exactly three strings, that
// disk_cache_size is bytes and belongs on screen in megabytes, that
// history_limit takes hold at the next history fetch rather than at once. What
// reflection is kept for is noticing a field nobody has described - see
// TestRegistry_CoversEveryConfigField and ADR 0008.
var registry = []settings.Entry{
	{
		Key:      "telegram.api_id",
		Group:    "telegram",
		Label:    "API ID",
		Help:     "The application this client signs in as, issued at my.telegram.org. Changing it is signing in as a different application, not editing a value.",
		Widget:   settings.Text,
		Applies:  settings.Startup,
		ReadOnly: true,
	},
	{
		Key:      "telegram.api_hash",
		Group:    "telegram",
		Label:    "API hash",
		Help:     "The secret paired with the API ID. It is shown masked because it is a credential and this screen ends up in recordings.",
		Widget:   settings.Text,
		Applies:  settings.Startup,
		ReadOnly: true,
		Secret:   true,
	},
	{
		Key:      "telegram.session_file",
		Group:    "telegram",
		Label:    "Session file",
		Help:     "Where the signed-in session is kept. Derived from the state directory unless the config pinned a path of its own; deprecated in favour of state_dir.",
		Widget:   settings.Text,
		Applies:  settings.Startup,
		ReadOnly: true,
	},
	{
		Key: "state_dir",
		// The file's root has no section name. The overlay titles this group
		// after the file itself.
		Group:    "",
		Label:    "State directory",
		Help:     "Where this account's state lives: the session, the local database and the lock that keeps a second instance out. Moving it is an account migration - files move and the session reopens - so it is a decision of its own rather than a field in a list.",
		Widget:   settings.Text,
		Applies:  settings.Startup,
		ReadOnly: true,
	},
	{
		Key:     "ui.theme",
		Group:   "ui",
		Label:   "Theme",
		Help:    "The theme to use. A name uses that theme against either terminal background; a dark/light pair uses one per background. Written the way you wrote it: naming one theme stays one row, naming a pair stays two.",
		Widget:  settings.Text,
		Applies: settings.Immediate,
	},
	{
		Key:     "ui.history_limit",
		Group:   "ui",
		Label:   "History limit",
		Help:    "How many messages are asked for when a chat is opened or scrolled back. What is already loaded stays loaded; the new value is what the next fetch asks for.",
		Widget:  settings.Number,
		Applies: settings.NextUse,
		Unit:    "messages",
		Min:     1,
		Max:     500,
	},
	{
		Key:     "ui.notification_preview",
		Group:   "ui",
		Label:   "Notification preview",
		Help:    "Whether a desktop notification carries the message text. Off sends the sender's name and nothing else.",
		Widget:  settings.Toggle,
		Applies: settings.Immediate,
	},
	{
		Key:     "ui.toasts.error_zone",
		Group:   "ui.toasts",
		Label:   "Error zone",
		Help:    "The corner errors appear in.",
		Widget:  settings.Choice,
		Applies: settings.Immediate,
		Choices: []string{"bottom-right", "top-right", "bottom-left"},
	},
	{
		Key:     "ui.toasts.notify_zone",
		Group:   "ui.toasts",
		Label:   "Notification zone",
		Help:    "The corner notifications appear in. The same corner as errors is allowed; they then stack together.",
		Widget:  settings.Choice,
		Applies: settings.Immediate,
		Choices: []string{"bottom-right", "top-right", "bottom-left"},
	},
	{
		Key:     "ui.toasts.max_visible",
		Group:   "ui.toasts",
		Label:   "Max visible",
		Help:    "How many toasts a corner shows at once. The rest are counted rather than drawn.",
		Widget:  settings.Number,
		Applies: settings.Immediate,
		Unit:    "toasts",
		Min:     1,
		Max:     10,
	},
	{
		Key:     "photos.eager_full_quality",
		Group:   "photos",
		Label:   "Eager full quality",
		Help:    "Whether opening a chat fetches full-resolution photos in the background, so opening one is instant at the cost of traffic.",
		Widget:  settings.Toggle,
		Applies: settings.Immediate,
	},
	{
		Key:     "photos.mode",
		Group:   "photos",
		Label:   "Image mode",
		Help:    "How images are drawn: auto picks Kitty graphics when the terminal supports them and block art otherwise. Read once at startup, because the renderer and everything already transmitted to the terminal are chosen from it.",
		Widget:  settings.Choice,
		Applies: settings.Startup,
		Choices: []string{"auto", "kitty", "blocks"},
	},
	{
		Key:     "photos.kitty_placement_cap",
		Group:   "photos",
		Label:   "Kitty placement cap",
		Help:    "How many Kitty image placements stay on the terminal at once. A whole heavy chat exceeds the terminal's own limit and corrupts placements, so only on-screen images and a few recently scrolled past stay transmitted. Lower it if images still corrupt.",
		Widget:  settings.Number,
		Applies: settings.NextUse,
		Unit:    "placements",
		Min:     1,
		Max:     256,
	},
	{
		Key:     "photos.max_long_side_px",
		Group:   "photos",
		Label:   "Max long side",
		Help:    "The longest side a rendered inline image is drawn at. Height is additionally bounded to a fraction of the chat pane. What is already drawn keeps the size it was drawn at.",
		Widget:  settings.Number,
		Applies: settings.NextUse,
		Unit:    "px",
		Min:     64,
		Max:     4096,
	},
	{
		Key:     "photos.disk_cache_size",
		Group:   "photos",
		Label:   "Media cache size",
		Help:    "How much of the on-disk media cache is kept between runs, so a chat re-renders its images instantly on restart. Zero keeps nothing: the cache moves into a temporary directory and is deleted on exit.",
		Widget:  settings.Bytes,
		Applies: settings.Startup,
		Min:     0,
	},
	{
		Key:     "avatars.disk_cache_size",
		Group:   "avatars",
		Label:   "Avatar cache size",
		Help:    "How much of the on-disk avatar cache is kept between runs. Budgeted apart from chat media so a scrolling session cannot evict the faces of people you talk to.",
		Widget:  settings.Bytes,
		Applies: settings.Startup,
		Min:     0,
	},
}

// excluded names a field of Config that is deliberately not a setting, and says
// why. It is part of the contract rather than an escape hatch: a field lands
// here with a reason, and a reason is expected to name an issue.
type excluded struct {
	Key    string
	Reason string
}

var exclusions = []excluded{
	{
		Key: "ui.date_format",
		Reason: "Read by nothing: the key is parsed into Config and dropped, and message timestamps use a hardcoded layout. " +
			"Declaring it would put a knob on screen that turns nothing. Implemented by #238, and this exclusion goes when it is.",
	},
	{
		Key: "keybindings",
		Reason: "Open by construction - the entries are whatever the user wrote - so there is no fixed list to declare. " +
			"Rebinding is also a different interaction from every widget here: capture a chord, report a collision, show the default it overrides. " +
			"Shown read-only in the overlay; edited by #241.",
	},
}

// repairIllegal checks every editable setting against its declaration and puts
// a value the app will not accept back to its default, before anything is
// unmarshalled into Config.
//
// Legality is declared once and used twice. The overlay refuses an illegal
// value at the widget; this refuses the same value in the file. Otherwise the
// two disagree, and the promise that editing by hand and editing in the overlay
// are the same act is only true for values that happen to be legal.
//
// It repairs rather than refuses to start: one wrong key should not keep a
// person out of their messages. What it replaces is a silent fallback deep in
// the UI, where the value was quietly swapped and nobody was told.
func repairIllegal(v *viper.Viper) []Warning {
	defaults := defaultValues()
	var warns []Warning
	for _, e := range registry {
		if e.ReadOnly {
			continue
		}
		err := e.Validate(v.Get(e.Key))
		if err == nil {
			continue
		}
		def, ok := defaults[e.Key]
		if !ok {
			// No default to fall back to. Say so and leave the value alone;
			// whatever reads it is where the fallback lives.
			warns = append(warns, Warning{Text: fmt.Sprintf("%s: %v; ignored", e.Key, err)})
			continue
		}
		v.Set(e.Key, def)
		warns = append(warns, Warning{Text: fmt.Sprintf("%s: %v; using %v instead", e.Key, err, def)})
	}
	return warns
}

// Settings returns the declared settings kept in the config file, in display
// order. The entries must be treated as read-only.
func Settings() []settings.Entry { return slices.Clone(registry) }

// Setting returns the declaration for a key.
func Setting(key string) (settings.Entry, bool) {
	i := slices.IndexFunc(registry, func(e settings.Entry) bool { return e.Key == key })
	if i < 0 {
		return settings.Entry{}, false
	}
	return registry[i], true
}
