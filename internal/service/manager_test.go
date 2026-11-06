package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/service"
)

// commands is a service manager's command line as a test plays it: every
// command it was given, and what each one answers. A command whose line
// starts with a key of fail fails.
type commands struct {
	ran  []string
	fail map[string]error
	// script answers a command whose line starts with its key, one answer per
	// call, the last one repeated; it is asked before fail.
	script map[string][]error
	// read keeps, for a command naming a file, what the file said then.
	read map[string]string
	// file is the argument after which a command names a file to read.
	file string
}

func (c *commands) run(name string, args ...string) error {
	line := strings.Join(append([]string{name}, args...), " ")
	c.ran = append(c.ran, line)
	for i, a := range args {
		if a == c.file && i+1 < len(args) {
			b, _ := os.ReadFile(args[i+1])
			if c.read == nil {
				c.read = map[string]string{}
			}
			c.read[line] = string(b)
		}
	}
	for prefix, answers := range c.script {
		if strings.HasPrefix(line, prefix) && len(answers) > 0 {
			err := answers[0]
			if len(answers) > 1 {
				c.script[prefix] = answers[1:]
			}
			return err
		}
	}
	for prefix, err := range c.fail {
		if strings.HasPrefix(line, prefix) {
			return err
		}
	}
	return nil
}

// clock is time as a test plays it: it stands still until something sleeps.
type clock struct {
	at    time.Time
	slept []time.Duration
}

func (c *clock) now() time.Time { return c.at }

func (c *clock) sleep(d time.Duration) {
	c.slept = append(c.slept, d)
	c.at = c.at.Add(d)
}

// errNoLabel is what launchctl print says of a label launchd does not hold.
var errNoLabel = errors.New(`launchctl: could not find service "io.github.sorokin-vladimir.tele" in domain for user gui: 501`)

var mine = service.Unit{Name: "tele", Binary: "/opt/homebrew/bin/tele", Config: "/Users/ada/.config/tele/config.yml"}

// A LaunchAgent is written where launchd reads the person's own, and loaded
// into their session; the log it names has a directory to go to (#182).
func TestLaunchd_InstallWritesTheAgentAndLoadsIt(t *testing.T) {
	home := t.TempDir()
	cmds := &commands{}
	m := service.Launchd{Home: home, UID: 501, Run: cmds.run}
	u := mine
	u.LogPath = filepath.Join(home, ".local", "state", "tele", "service.log")

	require.NoError(t, m.Install(u))

	plist := filepath.Join(home, "Library", "LaunchAgents", "io.github.sorokin-vladimir.tele.plist")
	written, err := os.ReadFile(plist)
	require.NoError(t, err)
	assert.Equal(t, string(service.Plist(u)), string(written))
	assert.Equal(t, []string{"launchctl bootstrap gui/501 " + plist}, cmds.ran)
	assert.DirExists(t, filepath.Dir(u.LogPath))
}

// Installing again replaces the agent and starts it anew: the way to mend a
// unit whose command moved.
func TestLaunchd_InstallingAgainReplacesTheAgent(t *testing.T) {
	home := t.TempDir()
	m := service.Launchd{Home: home, UID: 501, Run: (&commands{}).run}
	require.NoError(t, m.Install(mine))
	cmds := &commands{fail: map[string]error{"launchctl bootout": errors.New("not loaded"), "launchctl print": errNoLabel}}
	m.Run = cmds.run

	require.NoError(t, m.Install(mine))

	plist := filepath.Join(home, "Library", "LaunchAgents", "io.github.sorokin-vladimir.tele.plist")
	assert.Equal(t, []string{
		"launchctl bootout gui/501/io.github.sorokin-vladimir.tele",
		"launchctl print gui/501/io.github.sorokin-vladimir.tele",
		"launchctl bootstrap gui/501 " + plist,
	}, cmds.ran)
}

// Over a loaded agent, the new one is loaded only once launchd has let the old
// one's label go: it does so only when the old process has exited, and until
// then bootstrap fails with error 5 and leaves no service at all (#327).
func TestLaunchd_InstallWaitsForTheOldAgentToGo(t *testing.T) {
	home := t.TempDir()
	tick := &clock{}
	m := service.Launchd{Home: home, UID: 501, Run: (&commands{}).run, Sleep: tick.sleep, Now: tick.now}
	require.NoError(t, m.Install(mine))
	cmds := &commands{script: map[string][]error{"launchctl print": {nil, nil, nil, errNoLabel}}}
	m.Run = cmds.run

	require.NoError(t, m.Install(mine))

	plist := filepath.Join(home, "Library", "LaunchAgents", "io.github.sorokin-vladimir.tele.plist")
	assert.Equal(t, []string{
		"launchctl bootout gui/501/io.github.sorokin-vladimir.tele",
		"launchctl print gui/501/io.github.sorokin-vladimir.tele",
		"launchctl print gui/501/io.github.sorokin-vladimir.tele",
		"launchctl print gui/501/io.github.sorokin-vladimir.tele",
		"launchctl print gui/501/io.github.sorokin-vladimir.tele",
		"launchctl bootstrap gui/501 " + plist,
	}, cmds.ran)
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 100 * time.Millisecond, 100 * time.Millisecond}, tick.slept)
}

// An old agent that outlasts the wait still gets one try at loading the new
// one; when launchd refuses it, the error says why (#327).
func TestLaunchd_AnOldAgentThatStaysIsSaid(t *testing.T) {
	home := t.TempDir()
	tick := &clock{}
	m := service.Launchd{Home: home, UID: 501, Run: (&commands{}).run, Sleep: tick.sleep, Now: tick.now}
	require.NoError(t, m.Install(mine))
	cmds := &commands{fail: map[string]error{"launchctl bootstrap": errors.New("Bootstrap failed: 5: Input/output error")}}
	m.Run = cmds.run

	err := m.Install(mine)

	require.ErrorIs(t, err, service.ErrStillRunning)
	assert.Contains(t, err.Error(), "the running service did not stop in 30s")
	assert.Contains(t, err.Error(), "Input/output error")
	assert.Equal(t, 1, strings.Count(strings.Join(cmds.ran, "\n"), "launchctl bootstrap"))
	assert.Equal(t, 30*time.Second, tick.at.Sub(time.Time{}))
}

// What launchd said when it would not load the agent is the error.
func TestLaunchd_ARefusalIsSaid(t *testing.T) {
	cmds := &commands{fail: map[string]error{"launchctl bootstrap": errors.New("Bootstrap failed: 5: Input/output error")}}
	m := service.Launchd{Home: t.TempDir(), UID: 501, Run: cmds.run}

	err := m.Install(mine)

	assert.ErrorContains(t, err, "Input/output error")
}

// Uninstalling stops the agent and removes it; with nothing installed there is
// nothing to do, and that is no failure.
func TestLaunchd_Uninstall(t *testing.T) {
	home := t.TempDir()
	m := service.Launchd{Home: home, UID: 501, Run: (&commands{}).run}
	require.NoError(t, m.Install(mine))
	cmds := &commands{fail: map[string]error{"launchctl print": errNoLabel}}
	m.Run = cmds.run

	removed, err := m.Uninstall("tele")

	require.NoError(t, err)
	assert.True(t, removed)
	assert.Equal(t, []string{
		"launchctl bootout gui/501/io.github.sorokin-vladimir.tele",
		"launchctl print gui/501/io.github.sorokin-vladimir.tele",
	}, cmds.ran)
	assert.NoFileExists(t, filepath.Join(home, "Library", "LaunchAgents", "io.github.sorokin-vladimir.tele.plist"))

	removed, err = m.Uninstall("tele")
	require.NoError(t, err)
	assert.False(t, removed)
}

// Uninstalling returns only once launchd has let the agent go, so that the
// service is gone when it says so, and an install right after it loads (#327).
func TestLaunchd_UninstallWaitsForTheAgentToGo(t *testing.T) {
	home := t.TempDir()
	tick := &clock{}
	m := service.Launchd{Home: home, UID: 501, Run: (&commands{}).run, Sleep: tick.sleep, Now: tick.now}
	require.NoError(t, m.Install(mine))
	cmds := &commands{script: map[string][]error{"launchctl print": {nil, nil, errNoLabel}}}
	m.Run = cmds.run

	removed, err := m.Uninstall("tele")

	require.NoError(t, err)
	assert.True(t, removed)
	assert.Equal(t, []string{
		"launchctl bootout gui/501/io.github.sorokin-vladimir.tele",
		"launchctl print gui/501/io.github.sorokin-vladimir.tele",
		"launchctl print gui/501/io.github.sorokin-vladimir.tele",
		"launchctl print gui/501/io.github.sorokin-vladimir.tele",
	}, cmds.ran)
	assert.Len(t, tick.slept, 2)
}

// An agent that outlasts the wait is no reason to keep its file, but it is a
// reason not to say it is gone (#327).
func TestLaunchd_UninstallSaysAnAgentThatStays(t *testing.T) {
	home := t.TempDir()
	tick := &clock{}
	m := service.Launchd{Home: home, UID: 501, Run: (&commands{}).run, Sleep: tick.sleep, Now: tick.now}
	require.NoError(t, m.Install(mine))

	removed, err := m.Uninstall("tele")

	require.ErrorIs(t, err, service.ErrStillRunning)
	assert.True(t, removed)
	assert.NoFileExists(t, filepath.Join(home, "Library", "LaunchAgents", "io.github.sorokin-vladimir.tele.plist"))
}

// Whoever installs is told when a wait starts and when it ends, once each,
// and not at all when launchd let the old agent go at once (#327).
func TestLaunchd_AWaitIsTold(t *testing.T) {
	home := t.TempDir()
	tick := &clock{}
	var started, ended int
	m := service.Launchd{Home: home, UID: 501, Run: (&commands{}).run, Sleep: tick.sleep, Now: tick.now,
		Waiting: func() func() {
			started++
			return func() { ended++ }
		}}
	require.NoError(t, m.Install(mine))

	m.Run = (&commands{fail: map[string]error{"launchctl print": errNoLabel}}).run
	require.NoError(t, m.Install(mine))
	assert.Equal(t, 0, started, "nothing was waited for")

	m.Run = (&commands{script: map[string][]error{"launchctl print": {nil, nil, errNoLabel}}}).run
	require.NoError(t, m.Install(mine))
	assert.Equal(t, 1, started)
	assert.Equal(t, 1, ended)
}

// What an installed agent runs is read back from it, for status to check the
// command is still there and still this tele (#182).
func TestLaunchd_InstalledSaysWhatTheAgentRuns(t *testing.T) {
	m := service.Launchd{Home: t.TempDir(), UID: 501, Run: (&commands{}).run}
	_, ok, err := m.Installed("tele")
	require.NoError(t, err)
	assert.False(t, ok, "nothing installed")

	require.NoError(t, m.Install(service.Unit{Name: "tele", Binary: "/opt/home brew/bin/tele", Config: "/c.yml"}))

	binary, ok, err := m.Installed("tele")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "/opt/home brew/bin/tele", binary)
}

func TestSystemd_InstalledSaysWhatTheUnitRuns(t *testing.T) {
	m := service.Systemd{ConfigHome: t.TempDir(), Run: (&commands{}).run}
	_, ok, err := m.Installed("tele")
	require.NoError(t, err)
	assert.False(t, ok, "nothing installed")

	require.NoError(t, m.Install(service.Unit{Name: "tele", Binary: `/home/ada/my "bin"/tele%1`, Config: "/c.yml"}))

	binary, ok, err := m.Installed("tele")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, `/home/ada/my "bin"/tele%1`, binary)
}

// schtasks prints a task's XML; what it runs is read from that, whichever
// encoding it was printed in.
func TestTaskScheduler_InstalledSaysWhatTheTaskRuns(t *testing.T) {
	win := service.Unit{Name: "tele", Binary: `C:\Users\ada\bin\tele.exe`, Config: `C:\c.yml`}
	utf16 := service.TaskXML(win)
	utf8 := strings.Replace(string(decodeUTF16(utf16)), `encoding="UTF-16"`, `encoding="UTF-8"`, 1)
	for name, printed := range map[string][]byte{"UTF-16": utf16, "UTF-8": []byte(utf8)} {
		t.Run(name, func(t *testing.T) {
			m := service.TaskScheduler{Output: func(string, ...string) ([]byte, error) { return printed, nil }}

			binary, ok, err := m.Installed("tele")

			require.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, `C:\Users\ada\bin\tele.exe`, binary)
		})
	}

	none := service.TaskScheduler{Output: func(string, ...string) ([]byte, error) {
		return nil, errors.New("ERROR: The system cannot find the file specified.")
	}}
	_, ok, err := none.Installed("tele")
	require.NoError(t, err)
	assert.False(t, ok)
}

// decodeUTF16 reads little-endian UTF-16 with a byte order mark.
func decodeUTF16(b []byte) string {
	units := make([]uint16, 0, len(b)/2)
	for i := 2; i+1 < len(b); i += 2 {
		units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(units))
}

// A systemd user unit is written where the user's manager reads it, enabled
// for the next login, and started now - again, when it was running.
func TestSystemd_InstallWritesTheUnitAndStartsIt(t *testing.T) {
	config := t.TempDir()
	cmds := &commands{}
	m := service.Systemd{ConfigHome: config, Run: cmds.run}

	require.NoError(t, m.Install(mine))

	written, err := os.ReadFile(filepath.Join(config, "systemd", "user", "tele.service"))
	require.NoError(t, err)
	assert.Equal(t, string(service.SystemdUnit(mine)), string(written))
	assert.Equal(t, []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable tele.service",
		"systemctl --user restart tele.service",
	}, cmds.ran)
}

func TestSystemd_Uninstall(t *testing.T) {
	config := t.TempDir()
	m := service.Systemd{ConfigHome: config, Run: (&commands{}).run}
	require.NoError(t, m.Install(mine))
	cmds := &commands{}
	m.Run = cmds.run

	removed, err := m.Uninstall("tele")

	require.NoError(t, err)
	assert.True(t, removed)
	assert.Equal(t, []string{
		"systemctl --user disable --now tele.service",
		"systemctl --user daemon-reload",
	}, cmds.ran)
	assert.NoFileExists(t, filepath.Join(config, "systemd", "user", "tele.service"))

	removed, err = m.Uninstall("tele")
	require.NoError(t, err)
	assert.False(t, removed)
}

// A task is created from its XML, replacing one of the same name, and started
// now; one still running is ended first, so it starts on the new command.
func TestTaskScheduler_InstallCreatesTheTaskAndRunsIt(t *testing.T) {
	cmds := &commands{file: "/XML", fail: map[string]error{"schtasks /End": errors.New("not running")}}
	m := service.TaskScheduler{TempDir: t.TempDir(), Run: cmds.run}

	require.NoError(t, m.Install(mine))

	require.Len(t, cmds.ran, 3)
	assert.Equal(t, "schtasks /End /TN tele", cmds.ran[0])
	assert.True(t, strings.HasPrefix(cmds.ran[1], "schtasks /Create /TN tele /XML "), cmds.ran[1])
	assert.True(t, strings.HasSuffix(cmds.ran[1], " /F"), cmds.ran[1])
	assert.Equal(t, string(service.TaskXML(mine)), cmds.read[cmds.ran[1]], "the task is created from the unit")
	assert.Equal(t, "schtasks /Run /TN tele", cmds.ran[2])
}

// Uninstalling ends the task and deletes it; with no task there is nothing to
// do.
func TestTaskScheduler_Uninstall(t *testing.T) {
	cmds := &commands{}
	m := service.TaskScheduler{TempDir: t.TempDir(), Run: cmds.run}

	removed, err := m.Uninstall("tele")

	require.NoError(t, err)
	assert.True(t, removed)
	assert.Equal(t, []string{
		"schtasks /Query /TN tele",
		"schtasks /End /TN tele",
		"schtasks /Delete /TN tele /F",
	}, cmds.ran)

	none := &commands{fail: map[string]error{"schtasks /Query": errors.New("ERROR: The system cannot find the file specified.")}}
	m.Run = none.run
	removed, err = m.Uninstall("tele")
	require.NoError(t, err)
	assert.False(t, removed)
	assert.Equal(t, []string{"schtasks /Query /TN tele"}, none.ran)
}
