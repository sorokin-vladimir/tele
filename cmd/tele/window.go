package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/sorokin-vladimir/tele/internal/hosting"
)

// runWindow is tele with no command: a window on the account, hosting it when
// nobody else does.
func runWindow(o options, themeCheck, themeDump optionalArg) error {
	s, err := newSetup(o.cfgPath)
	if err != nil {
		return err
	}
	cfg := s.store.Current()
	themesDir, themes, warnings := loadThemes(cfg)

	if themeCheck.set {
		fmt.Print(themeReport(themesDir, themes, themeCheck.value, warnings))
		return nil
	}
	if themeDump.set {
		out, err := themeDumpText(themesDir, themes, themeDump.value)
		if err != nil {
			return fmt.Errorf("theme: %w", err)
		}
		fmt.Print(out)
		return nil
	}

	key, err := s.appKey()
	if err != nil {
		return err
	}
	if err := s.makeLogDir(); err != nil {
		return err
	}
	// Every start begins by reaching the host of the state directory: a tele
	// already holding it is attached to, and with nobody there this one
	// becomes the host, and then reaches itself the same way (#208).
	reached, err := hosting.Reach(context.Background(), appName, cfg.StateDir)
	if err != nil {
		return errors.New(reachFailure(err, cfg.StateDir))
	}
	isHost := reached.Listener != nil
	log, logPath, closeLog := openLog(s.logDir, isHost, o.verbose)
	defer func() {
		_ = log.Sync()
		_ = closeLog()
	}()

	// The log keeps every warning on every run, including the ones the TUI shows
	// only once: suppressing a toast must not lose the record.
	for _, w := range warnings {
		log.Warn("config: " + w.Text)
		fmt.Fprintf(os.Stderr, "config: %s\n", w.Text)
	}

	r := run{
		cfgStore:      s.store,
		cfgPath:       s.cfgPath,
		key:           key,
		log:           log,
		logPath:       logPath,
		verbose:       o.verbose,
		trace:         o.trace,
		themeWarnings: themes.Warnings,
		reached:       reached,
	}
	if isHost {
		return r.host()
	}
	return r.client()
}
