package config

import "github.com/spf13/viper"

func setDefaults(v *viper.Viper) {
	v.SetDefault("ui.date_format", "15:04")
	v.SetDefault("ui.history_limit", 50)
	v.SetDefault("ui.theme", "default")
}
