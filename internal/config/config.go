package config

import "github.com/spf13/viper"

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

type Config struct {
	Telegram TelegramConfig `mapstructure:"telegram"`
	UI       UIConfig       `mapstructure:"ui"`
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
	return &cfg, nil
}
