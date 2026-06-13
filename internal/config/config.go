package config

import (
	"path/filepath"

	"github.com/spf13/viper"
)

type TelegramConfig struct {
	APIID       int    `mapstructure:"api_id"`
	APIHash     string `mapstructure:"api_hash"`
	SessionFile string `mapstructure:"session_file"`
}

type UIConfig struct {
	Theme        string `mapstructure:"theme"`
	DateFormat   string `mapstructure:"date_format"`
	HistoryLimit int    `mapstructure:"history_limit"`
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
}

type Config struct {
	Telegram    TelegramConfig            `mapstructure:"telegram"`
	UI          UIConfig                  `mapstructure:"ui"`
	Photos      PhotosConfig              `mapstructure:"photos"`
	Keybindings map[string]map[string]any `mapstructure:"keybindings"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	setDefaults(v)
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if cfg.Telegram.SessionFile == "" {
		cfg.Telegram.SessionFile = filepath.Join(filepath.Dir(path), "session.json")
	}
	return &cfg, nil
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
