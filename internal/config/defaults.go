package config

import (
	"github.com/spf13/viper"

	"github.com/sorokin-vladimir/tele/internal/proxy"
)

// defaultValues is what a setting is worth when the file does not name it, and
// what an illegal value in the file is repaired to. One map rather than a run of
// SetDefault calls, because repair needs to read a default back after the file
// has been loaded over it.
//
// ui.date_format is deliberately absent: nothing reads it, so a default for it
// would be a promise the app does not keep. See #238 and the registry
// exclusions.
func defaultValues() map[string]any {
	return map[string]any{
		// auto is the file saying nothing about the route, which is not the
		// same as saying direct: it leaves ALL_PROXY in charge, the way tele
		// has since v1.3.0. The port has no default that could be right - any
		// number here would be a port nobody chose, silently dialled - so zero
		// stands for "not set" and proxy.Parse refuses it when the type needs
		// one.
		"proxy.type":                 proxy.TypeAuto,
		"proxy.port":                 0,
		"ui.history_limit":           50,
		"ui.confirm_quit":            false,
		"ui.notifications.desktop":   true,
		"ui.notifications.toast":     true,
		"ui.notifications.preview":   true,
		"ui.toasts.error_zone":       "bottom-right",
		"ui.toasts.notify_zone":      "top-right",
		"ui.toasts.max_visible":      3,
		"photos.eager_full_quality":  true,
		"photos.mode":                "auto",
		"photos.kitty_placement_cap": 16,
		"photos.max_long_side_px":    800,
		"photos.disk_cache_size":     int64(256 * 1024 * 1024),
		"avatars.disk_cache_size":    int64(16 * 1024 * 1024),
	}
}

func setDefaults(v *viper.Viper) {
	for key, value := range defaultValues() {
		v.SetDefault(key, value)
	}
}
