package config

import "github.com/spf13/viper"

func setDefaults(v *viper.Viper) {
	v.SetDefault("ui.date_format", "15:04")
	v.SetDefault("ui.history_limit", 50)
	v.SetDefault("ui.theme", "default")
	v.SetDefault("photos.eager_full_quality", true)
	v.SetDefault("photos.mode", "auto")
	v.SetDefault("photos.kitty_placement_cap", 16)
	v.SetDefault("photos.max_long_side_px", 800)
}
