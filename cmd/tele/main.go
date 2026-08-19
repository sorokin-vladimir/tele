package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/sorokin-vladimir/tele/internal/app"
	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/statedir"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
	"github.com/sorokin-vladimir/tele/internal/version"
)

// Injected at build time via -ldflags. Fall back to config file values if zero.
var (
	buildAPIID   = "0"
	buildAPIHash = ""
	appName      = "tele" // injected via -ldflags for the beta channel
)

func main() {
	cfgPath := flag.String("config", defaultConfigPath(appName), "path to config file")
	verbose := flag.Bool("e", false, "debug logging")
	trace := flag.Bool("trace", false, "log sensitive metadata (peer IDs, message lengths) — never use in shared environments")
	versionFlag := flag.Bool("version", false, "print version and exit")
	var themeCheck, themeDump optionalArg
	flag.Var(&themeCheck, "theme-check", "print which theme each slot resolved to and where its tokens came from, then exit; give a theme name to inspect one")
	flag.Var(&themeDump, "theme-dump", "print a slot's theme (dark or light) as a complete theme file, then exit; give a theme name to dump one")
	flag.Parse()

	if *versionFlag {
		fmt.Println(version.Version)
		os.Exit(0)
	}

	expanded := config.ExpandTilde(*cfgPath)
	cfgPath = &expanded

	if err := ensureConfig(*cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	defaultState, err := stateDirPath(appName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "state dir: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(*cfgPath, defaultState)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	// Themes are resolved here, before the TUI exists, so a bad theme file is an
	// ordinary config warning rather than something the interface has to cope
	// with. The TUI is handed the result and never sees the config.
	themesDir := cfg.ThemesDir
	themes := theme.LoadSlots(themesDir, cfg.UI.ThemeSlots.Dark, cfg.UI.ThemeSlots.Light)
	// Theme problems repeat every launch: unlike a dead config key, they are
	// still true next time.
	for _, w := range themes.Warnings {
		cfg.Warnings = append(cfg.Warnings, config.Warning{Text: "theme: " + w})
	}
	theme.SetSlots(themes.Slots())

	if themeCheck.set {
		fmt.Print(themeReport(themesDir, themes, themeCheck.value, cfg.Warnings))
		os.Exit(0)
	}
	if themeDump.set {
		out, err := themeDumpText(themesDir, themes, themeDump.value)
		if err != nil {
			fmt.Fprintf(os.Stderr, "theme: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(out)
		os.Exit(0)
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

	// The log lives in the platform state home regardless of where the account
	// state was pinned: it is per-machine diagnostic output, not account data.
	if err := os.MkdirAll(defaultState, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "state dir: %v\n", err)
		os.Exit(1)
	}
	level := zap.NewAtomicLevelAt(zap.InfoLevel)
	if *verbose {
		level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}
	logPath := filepath.Join(defaultState, "tele.log")
	w := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10,
		MaxBackups: 3,
		Compress:   false,
	})
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		w,
		level,
	)
	// Every line is tagged with the release it came from: a log excerpt from a
	// user is otherwise impossible to attribute to a build.
	log := zap.New(core).With(zap.String("v", version.Version))
	defer log.Sync() //nolint:errcheck

	// The log keeps every warning on every run, including the ones the TUI shows
	// only once: suppressing a toast must not lose the record.
	for _, w := range cfg.Warnings {
		log.Warn("config: " + w.Text)
		fmt.Fprintf(os.Stderr, "config: %s\n", w.Text)
	}

	// Ownership of the state directory is taken before anything opens the
	// session or the database, and held for the process lifetime.
	lock, err := statedir.Acquire(cfg.StateDir)
	if err != nil {
		if errors.Is(err, statedir.ErrLocked) {
			fmt.Fprintf(os.Stderr,
				"tele is already running (%v).\nOnly one instance can use %s at a time.\n",
				err, cfg.StateDir)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "state dir: %v\n", err)
		os.Exit(1)
	}
	defer lock.Release() //nolint:errcheck

	// A pinned session_file is left exactly where the user put it, so nothing is
	// migrated in that case.
	stateMoved := false
	if !cfg.SessionPinned {
		stateMoved, err = statedir.Migrate(cfg.StateDir, filepath.Dir(*cfgPath), log)
		if err != nil {
			fmt.Fprintf(os.Stderr, "state migration: %v\n", err)
			os.Exit(1)
		}
	}

	a, err := app.New(cfg, log, *verbose, *trace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		os.Exit(1)
	}
	a.SetStateMoved(stateMoved)
	a.SetLogPath(logPath)
	if err := a.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

const defaultConfigHead = `telegram:
  api_id: 0
  api_hash: ""

ui:
  history_limit: 50
  # Themes follow the terminal background: a dark one and a light one, each
  # named here. Leave this out for the built-in tele-dark and tele-light. See
  # docs/themes.md.
  # theme:
  #   dark: my-dark
  #   light: my-light

photos:
  eager_full_quality: true  # download full resolution in background on chat open

# Keybindings — every action with its current default keys (see docs/keybindings.md).
# Uncomment a line and change its key(s) to override that action in that context.
# One key replaces the defaults; a chord is space-separated tokens ("g g" = g then g).
`

// defaultConfig is the head plus the generated, fully-commented keybindings
// reference, so the written config stays in sync with the actual defaults.
func defaultConfig() string {
	return defaultConfigHead + keys.DefaultKeybindingsYAML()
}

// defaultConfigPath returns the default config file location for the given app
// name, e.g. ~/.config/tele/config.yml (or ~/.config/tele-beta/config.yml for
// the beta channel). The tilde is expanded later by expandTilde.
func defaultConfigPath(app string) string {
	return filepath.Join("~", ".config", app, "config.yml")
}

// stateDirPath returns $XDG_STATE_HOME/<app>, falling back to
// ~/.local/state/<app>. It does not create the directory: the state directory
// is created under lock by statedir.Acquire, and the log directory is created
// by its own caller.
func stateDirPath(app string) (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, app), nil
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
	return os.WriteFile(path, []byte(defaultConfig()), 0600)
}
