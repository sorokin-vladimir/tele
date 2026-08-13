package config

import "github.com/spf13/viper"

func setDefaults(v *viper.Viper) {
	v.SetDefault("ui.date_format", "15:04")
	v.SetDefault("ui.history_limit", 50)
	v.SetDefault("ui.notification_preview", true)
	v.SetDefault("ui.toasts.error_zone", "bottom-right")
	v.SetDefault("ui.toasts.notify_zone", "top-right")
	v.SetDefault("ui.toasts.max_visible", 3)
	v.SetDefault("photos.eager_full_quality", true)
	v.SetDefault("photos.mode", "auto")
	v.SetDefault("photos.kitty_placement_cap", 16)
	v.SetDefault("photos.max_long_side_px", 800)
	v.SetDefault("photos.disk_cache_size", int64(256*1024*1024))
	v.SetDefault("avatars.disk_cache_size", int64(16*1024*1024))
}
