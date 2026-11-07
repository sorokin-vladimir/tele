package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runBuilt runs the built tele with args, in a home of its own, and returns
// what it printed and the code it exited with.
func runBuilt(t *testing.T, args ...string) (string, int) {
	t.Helper()
	bin := buildTele(t)
	isolatedHome(t)
	out, err := exec.Command(bin, args...).CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return string(out), 0
	case errors.As(err, &exit):
		return string(out), exit.ExitCode()
	default:
		t.Fatalf("running tele %v: %v", args, err)
		return "", 0
	}
}

// configAt writes a config whose state directory is stateDir, and returns its
// path.
func configAt(t *testing.T, stateDir string) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "work.yml")
	body := "telegram:\n  api_id: 1\n  api_hash: x\nstate_dir: " + stateDir + "\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// An action is a command, a flag only shapes one: the version is asked for by
// `tele version` (#213).
func TestVersion_IsACommand(t *testing.T) {
	out, code := runBuilt(t, "version")

	if code != 0 || strings.TrimSpace(out) != builtVersion {
		t.Errorf("tele version printed %q, exit %d", out, code)
	}
}

// --version still answers, for the homebrew-core formula test, but is not
// offered: the help names the command.
func TestVersion_TheFlagAnswersUnoffered(t *testing.T) {
	help, _ := runBuilt(t, "--help")

	assertSays(t, help, "version")
	if strings.Contains(help, "--version") {
		t.Errorf("the help offers --version:\n%s", help)
	}
}

// The theme tools are commands too: `tele theme dump` writes a theme out as a
// file, `tele theme check` says what each slot resolved to (#213).
func TestTheme_DumpAndCheckAreCommands(t *testing.T) {
	dump, code := runBuilt(t, "theme", "dump", "tele-dark")
	if code != 0 {
		t.Fatalf("tele theme dump exited %d:\n%s", code, dump)
	}
	assertSays(t, dump, "dumped from tele-dark")

	check, code := runBuilt(t, "theme", "check")
	if code != 0 {
		t.Fatalf("tele theme check exited %d:\n%s", code, check)
	}
	assertSays(t, check, "themes directory:", "dark", "light")
}

// The flags still answer, for the homebrew-core formula test, but are not
// offered.
func TestTheme_TheFlagsAnswerUnoffered(t *testing.T) {
	dump, code := runBuilt(t, "--theme-check")
	if code != 0 {
		t.Fatalf("tele --theme-check exited %d:\n%s", code, dump)
	}
	help, _ := runBuilt(t, "--help")
	if strings.Contains(help, "--theme-") {
		t.Errorf("the help offers the theme flags:\n%s", help)
	}
}

// A command line tele cannot read exits 2, whatever is wrong with it, so a
// script tells a mistake of its own from anything that went wrong (#213).
func TestUsage_AMistakeExitsTwo(t *testing.T) {
	for _, args := range [][]string{
		{"--no-such-flag"},
		{"no-such-command"},
		{"version", "extra"},
		{"daemon", "status", "--no-such-flag"},
		{"chats", "extra"},
		{"chats", "--archived", "--folder", "3"},
		{"chats", "--limit", "0"},
		{"topics"},
		{"topics", "@gophers", "--limit", "0"},
		{"topics", "Gophers"},
		{"show"},
		{"show", "Bob"},
		{"show", "@bob", "--limit", "0"},
		{"show", "@bob", "--limit", "201"},
		{"send"},
		{"send", "Bob", "hi"},
		{"send", "@bob"},
		{"send", "@bob", "hi", "--timeout", "0"},
	} {
		out, code := runBuilt(t, args...)
		if code != usageCode {
			t.Errorf("tele %v exited %d:\n%s", args, code, out)
		}
	}
}

// A command that acts on the account asks the tele holding it and never
// starts one: with none, it exits 3 and says how to get one (#181).
func TestChats_WithNoTeleExitsThree(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "work-state")
	cfg := configAt(t, stateDir)

	out, code := runBuilt(t, "-c", cfg, "chats")

	if code != noHostCode {
		t.Errorf("exited %d:\n%s", code, out)
	}
	assertSays(t, out, stateDir, "tele daemon install")
}

// -c is --config, the one flag typed by hand often enough for a short form
// (#213).
func TestFlags_CIsTheConfig(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "work-state")
	cfg := configAt(t, stateDir)

	out, code := runBuilt(t, "-c", cfg, "daemon", "status")

	assertSays(t, out, stateDir)
	if code != noHostCode {
		t.Errorf("exited %d", code)
	}
}
