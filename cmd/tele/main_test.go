package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestDefaultConfigPath(t *testing.T) {
	if got, want := defaultConfigPath("tele"), filepath.Join("~", ".config", "tele", "config.yml"); got != want {
		t.Fatalf("stable: got %q want %q", got, want)
	}
	if got, want := defaultConfigPath("tele-beta"), filepath.Join("~", ".config", "tele-beta", "config.yml"); got != want {
		t.Fatalf("beta: got %q want %q", got, want)
	}
}

func TestStateDirPath_UsesXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/xdg/state")

	got, err := stateDirPath("tele")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/xdg/state", "tele"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestStateDirPath_FallsBackToHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	got, err := stateDirPath("tele-beta")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".local", "state", "tele-beta"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// stateDirPath must not create anything. Creation belongs to whoever owns the
// directory: statedir.Acquire for the state directory, main for the log
// directory.
func TestStateDirPath_DoesNotCreateTheDirectory(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)

	got, err := stateDirPath("tele-beta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatalf("directory should not exist, stat err = %v", err)
	}
}

// The homebrew-core formula builds tele from source and then runs two commands,
// matching their output: `tele --version`, and `tele --theme-dump=tele-dark` for
// something that does real work without a config file, a network or a terminal.
// It also injects the version through a -ldflags path that no compiler checks:
// name the wrong symbol and the build still succeeds, silently reporting "dev".
//
// Homebrew's CI is where all of that would otherwise break, on an autobump pull
// request nobody here opened, for a release that already shipped. Anything this
// test pins may only change together with a pull request to
// Homebrew/homebrew-core.
func TestHomebrewCoreContract(t *testing.T) {
	const wantVersion = builtVersion
	bin := buildTele(t)

	// The formula's own invocation: no --config, and whatever HOME the test
	// sandbox happens to have. Pointing HOME at a temporary directory keeps the
	// run off the developer's real config and themes.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))

	version := formulaOutput(t, bin, "--version")
	if got := strings.TrimSpace(version); got != wantVersion {
		t.Fatalf("tele --version printed %q, want %q: the formula matches the version it built with", got, wantVersion)
	}

	dump := formulaOutput(t, bin, "--theme-dump=tele-dark")
	if want := "dumped from tele-dark"; !strings.Contains(dump, want) {
		t.Fatalf("tele --theme-dump=tele-dark printed %q, want it to contain %q", dump, want)
	}
}

// formulaOutput runs bin the way the formula's shell_output does: it must exit
// 0, and only what it prints to standard output is matched. Text that moved to
// standard error would fail the formula while still being printed.
func formulaOutput(t *testing.T, bin string, args ...string) string {
	t.Helper()
	var stdout, stderr strings.Builder
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("tele %s: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// builtVersion is the version buildTele injects.
const builtVersion = "9.9.9-contract"

// buildTele builds this command, once for every test that runs it. It is built
// before a test moves HOME: the go command keeps its module and build caches
// under the home directory, and a temporary one would send it to fetch the
// whole module graph again.
func buildTele(t *testing.T) string {
	t.Helper()
	built.once.Do(func() {
		built.path = filepath.Join(built.dir, "tele")
		if runtime.GOOS == "windows" {
			built.path += ".exe"
		}
		out, err := exec.Command("go", "build",
			"-ldflags", "-X github.com/sorokin-vladimir/tele/internal/version.Version="+builtVersion,
			"-o", built.path, ".").CombinedOutput()
		if err != nil {
			built.err = fmt.Errorf("%v\n%s", err, out)
		}
	})
	if built.err != nil {
		t.Fatalf("building tele: %v", built.err)
	}
	return built.path
}

// built is the binary buildTele made, in a directory that outlives any one
// test and goes when they are all done.
var built struct {
	dir  string
	once sync.Once
	path string
	err  error
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tele-bin-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	built.dir = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
