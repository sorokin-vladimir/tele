package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type TelegramConfig struct {
	APIID       int    `mapstructure:"api_id"`
	APIHash     string `mapstructure:"api_hash"`
	SessionFile string `mapstructure:"session_file"`
}

// ThemeSlots names the theme to use against each terminal background. An empty
// name means the built-in for that slot.
type ThemeSlots struct {
	Dark, Light string
}

// themesDirName is the directory, alongside the config file, that theme files
// are read from.
const themesDirName = "themes"

type UIConfig struct {
	// Theme is kept as written because its two spellings mean different things:
	// a name puts that theme in both slots, a map fills the slots it names. Read
	// ThemeSlots instead; resolveTheme fills it.
	Theme        any        `mapstructure:"theme"`
	ThemeSlots   ThemeSlots `mapstructure:"-"`
	DateFormat   string     `mapstructure:"date_format"`
	HistoryLimit int        `mapstructure:"history_limit"`
	// NotificationPreview controls whether the message text is included in
	// desktop notifications. Set false to send only the sender name (#80).
	NotificationPreview bool         `mapstructure:"notification_preview"`
	Toasts              ToastsConfig `mapstructure:"toasts"`
}

// ToastsConfig controls the floating toast component (#87). Zone strings are
// "bottom-right", "top-right", or "bottom-left"; unknown values fall back to
// the default at the UI layer.
type ToastsConfig struct {
	ErrorZone  string `mapstructure:"error_zone"`
	NotifyZone string `mapstructure:"notify_zone"`
	MaxVisible int    `mapstructure:"max_visible"`
}

type PhotosConfig struct {
	EagerFullQuality bool   `mapstructure:"eager_full_quality"`
	Mode             string `mapstructure:"mode"` // auto | kitty | blocks
	// KittyPlacementCap bounds how many Kitty image placements are kept on the
	// terminal at once. Transmitting an entire heavy chat exceeds the terminal's
	// limit and corrupts placements, so only on-screen images (plus a few
	// recently scrolled-past) stay transmitted. Lower it if images still corrupt.
	KittyPlacementCap int `mapstructure:"kitty_placement_cap"`
	// MaxLongSidePx caps a rendered inline image's long side in pixels (mirrors
	// the desktop client's fixed media ceiling). Height is additionally bounded
	// to a fraction of the chat pane. Raise it for larger inline images.
	MaxLongSidePx int `mapstructure:"max_long_side_px"`
	// DiskCacheSize bounds the on-disk media cache in bytes. Fetched thumbnails,
	// stickers and voice notes are cached per account under the user cache
	// directory, so a chat re-renders its images instantly on restart. 0 means
	// keep nothing between runs: the cache moves into the process's temp
	// directory under a fixed bound and is deleted on exit. See issues #174 and
	// #196.
	DiskCacheSize int64 `mapstructure:"disk_cache_size"`
}

// AvatarsConfig controls people's pictures. It is separate from PhotosConfig
// because an avatar and a photo in a chat share nothing but being an image: an
// avatar is bounded in size, belongs to a person, and is fetched over and over
// for the same handful of people, so it gets its own budget rather than
// competing with scrolled-past video thumbnails (#223, ADR 0007).
type AvatarsConfig struct {
	// DiskCacheSize bounds the on-disk avatar cache in bytes. An avatar is
	// fetched at Telegram's big size (ADR 0011), around a hundred kilobytes, so
	// the default holds more people than a chat list shows in a sitting rather
	// than the several hundred it held when the picture was small.
	// 0 means keep nothing between runs, exactly as in
	// PhotosConfig: the cache moves into the process's temp directory under a
	// fixed bound and is deleted on exit.
	DiskCacheSize int64 `mapstructure:"disk_cache_size"`
}

type Config struct {
	Telegram    TelegramConfig            `mapstructure:"telegram"`
	UI          UIConfig                  `mapstructure:"ui"`
	Photos      PhotosConfig              `mapstructure:"photos"`
	Avatars     AvatarsConfig             `mapstructure:"avatars"`
	Keybindings map[string]map[string]any `mapstructure:"keybindings"`

	// StateDir holds one account's state: the session, the SQLite database and
	// the ownership lock. See resolveState for how it is chosen.
	StateDir string `mapstructure:"state_dir"`

	// ThemesDir holds the user's theme files. It sits beside the config rather
	// than in the state directory: a theme is something you edit, like the
	// config, not something the app maintains.
	ThemesDir string `mapstructure:"-"`

	// SessionPinned reports that telegram.session_file named a deliberate
	// location. The caller must not migrate files away from it.
	SessionPinned bool `mapstructure:"-"`

	// Warnings collects non-fatal config notices for the caller to log, in the
	// same spirit as keys.MergeOverrides.
	Warnings []Warning `mapstructure:"-"`

	// named records which settings the file itself names, as opposed to holding
	// their default. It is what lets a setting be shown as "default" rather than
	// as a choice somebody made, and it can only be known while the file is
	// open, which is why it is captured here rather than worked out later.
	named map[string]bool
}

// Warning is a non-fatal config notice.
//
// Most describe something that is still wrong — a theme that is not there, a
// color that did not parse — and belong on screen every launch, because every
// launch they are still true.
//
// One carrying an ID does not: it describes a key that is merely dead, where
// acting on it changes nothing and only tidies the file. Repeating that at every
// launch is nagging, so the ID keys a seen-state and it is shown once.
type Warning struct {
	Text string
	ID   string
}

// warn records a warning shown at every launch.
func (c *Config) warn(format string, args ...any) {
	c.Warnings = append(c.Warnings, Warning{Text: fmt.Sprintf(format, args...)})
}

// warnOnce records a warning shown only until the user has seen it.
func (c *Config) warnOnce(id, format string, args ...any) {
	c.Warnings = append(c.Warnings, Warning{Text: fmt.Sprintf(format, args...), ID: id})
}

// Load reads the config at path. defaultStateDir is the platform state
// directory, used when the config names none.
func Load(path, defaultStateDir string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	setDefaults(v)
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	repairs := repairIllegal(v)
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	cfg.Warnings = append(cfg.Warnings, repairs...)
	cfg.named = namedInFile(v)
	cfg.resolveState(defaultStateDir)
	cfg.ThemesDir = filepath.Join(filepath.Dir(path), themesDirName)
	cfg.resolveTheme()
	return &cfg, nil
}

// legacyThemeName is the value shipped in every config tele has ever written. It
// named a family holding both a dark and a light palette; themes no longer come
// in pairs, so nothing resolves by it and it is read as "leave both slots
// alone" — which is exactly what it used to do.
const legacyThemeName = "default"

// resolveTheme reads ui.theme, which is either a theme name or a map naming one
// per slot:
//
//	theme: gruvbox-dark          # this theme against either background
//	theme: {dark: g, light: s}   # one per background
//	theme: {dark: g}             # dark from g, light stays built-in
//
// The spelling carries the intent, so nothing has to be guessed. A bare name
// filling both slots is what makes "follow the terminal" need no off switch:
// both slots are always filled, and naming one theme fills them with the same
// one.
func (c *Config) resolveTheme() {
	switch v := c.UI.Theme.(type) {
	case nil:
		return
	case string:
		name := c.themeName(v)
		c.UI.ThemeSlots = ThemeSlots{Dark: name, Light: name}
	case map[string]any:
		for key, raw := range v {
			name, ok := raw.(string)
			if !ok {
				c.warn("ui.theme.%s must be a theme name; ignored", key)
				continue
			}
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "dark":
				c.UI.ThemeSlots.Dark = c.themeName(name)
			case "light":
				c.UI.ThemeSlots.Light = c.themeName(name)
			default:
				c.warn("ui.theme has no slot %q; the slots are dark and light", key)
			}
		}
	default:
		c.warn("ui.theme must be a theme name or a map of dark/light; ignored")
	}
}

// themeName normalizes one name out of ui.theme, turning the retired "default"
// into the empty name that means the built-in.
func (c *Config) themeName(s string) string {
	name := strings.TrimSpace(s)
	if name == "" {
		return ""
	}
	if strings.EqualFold(name, legacyThemeName) {
		// Shown once: the config already behaves the way it always did, and the
		// only thing left to do is delete a line. Nagging about that every
		// launch would be worse than the dead line itself.
		c.warnOnce("config.ui.theme.default",
			`ui.theme: "default" is no longer a theme name and is ignored — the built-in themes are tele-dark and tele-light. Nothing has changed for you; you can delete the line. See docs/themes.md`)
		return ""
	}
	return name
}

// resolveState fixes StateDir and Telegram.SessionFile. Precedence:
//
//  1. state_dir is canonical when set; a session_file alongside it is ignored.
//  2. telegram.session_file alone keeps its own directory as the state
//     directory and pins it. A deliberate path — an encrypted volume, an
//     external disk — must never be relocated behind the user's back.
//  3. neither: the platform state directory. Legacy files next to the config
//     are moved into it at startup by statedir.Migrate.
func (c *Config) resolveState(defaultStateDir string) {
	switch {
	case c.StateDir != "":
		c.StateDir = ExpandTilde(c.StateDir)
		if c.Telegram.SessionFile != "" {
			c.warn("telegram.session_file is ignored because state_dir is set; remove it from the config")
		}
	case c.Telegram.SessionFile != "":
		c.Telegram.SessionFile = ExpandTilde(c.Telegram.SessionFile)
		c.StateDir = filepath.Dir(c.Telegram.SessionFile)
		c.SessionPinned = true
		c.warn("telegram.session_file is deprecated and will be removed in the next release; set state_dir instead")
		return
	default:
		c.StateDir = defaultStateDir
	}
	c.Telegram.SessionFile = filepath.Join(c.StateDir, "session.json")
}

// ExpandTilde replaces a leading ~/ with the user home directory. Paths that do
// not start with ~/ are returned unchanged.
func ExpandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// KeybindingOverrides flattens the raw keybindings section into
// context -> action -> []key, normalizing scalar ("R") and sequence
// (["g g","gg"]) values. Returns nil when the section is absent.
// Exported because internal/app and external tests call it across packages.
func (c *Config) KeybindingOverrides() map[string]map[string][]string {
	if len(c.Keybindings) == 0 {
		return nil
	}
	out := make(map[string]map[string][]string, len(c.Keybindings))
	for ctx, actions := range c.Keybindings {
		m := make(map[string][]string, len(actions))
		for action, raw := range actions {
			m[action] = toStringSlice(raw)
		}
		out[ctx] = m
	}
	return out
}

func toStringSlice(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
