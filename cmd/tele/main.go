package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/app"
	"github.com/sorokin-vladimir/tele/internal/config"
)

// Injected at build time via -ldflags. Fall back to config file values if zero.
var (
	buildAPIID   = "0"
	buildAPIHash = ""
)

func main() {
	cfgPath := flag.String("config", "~/.config/tele/config.yml", "path to config file")
	flag.Parse()

	expanded := expandTilde(*cfgPath)
	cfgPath = &expanded

	if err := ensureConfig(*cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	if cfg.Telegram.APIID == 0 {
		if id, err := strconv.Atoi(buildAPIID); err == nil && id != 0 {
			cfg.Telegram.APIID = id
		}
	}
	if cfg.Telegram.APIHash == "" {
		cfg.Telegram.APIHash = buildAPIHash
	}

	if cfg.Telegram.APIID == 0 || cfg.Telegram.APIHash == "" {
		fmt.Fprintf(os.Stderr, "config: set telegram.api_id and telegram.api_hash in %s\nGet credentials at https://my.telegram.org\n", *cfgPath)
		os.Exit(1)
	}

	logCfg := zap.NewProductionConfig()
	logCfg.OutputPaths = []string{"tele.log"}
	log, err := logCfg.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck

	a := app.New(cfg, log)
	if err := a.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

const defaultConfig = `telegram:
  api_id: 0
  api_hash: ""

ui:
  date_format: "15:04"
  history_limit: 50
  theme: default
`

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// ensureConfig creates a default config file if it does not exist.
func ensureConfig(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(defaultConfig), 0600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created default config at %s\nEdit it and set telegram.api_id and telegram.api_hash (get them at https://my.telegram.org)\n", path)
	os.Exit(0)
	return nil
}
