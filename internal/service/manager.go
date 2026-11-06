package service

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// Manager registers units with the operating system's service manager.
type Manager interface {
	// Install writes u and starts the service. A unit already installed under
	// its name is replaced, and the service started anew on it once the old
	// one is gone; ErrStillRunning when that did not happen in time.
	Install(u Unit) error
	// Uninstall stops the service named name and removes its unit, and returns
	// once the service is gone; ErrStillRunning when it did not go in time. It
	// reports false, and does nothing, when none is installed.
	Uninstall(name string) (bool, error)
	// Installed is the command the service named name runs, as its unit
	// says; false when none is installed.
	Installed(name string) (binary string, ok bool, err error)
}

// Runner runs a command of the service manager's. Its error carries what the
// command said.
type Runner func(name string, args ...string) error

// Run runs a command of the service manager's for real.
func Run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if said := strings.TrimSpace(string(out)); said != "" {
			return fmt.Errorf("%s: %s", name, said)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Output runs a command of the service manager's for real, for what it
// prints.
func Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// ErrUnsupported is a system tele has no service manager for. A host is run
// there by hand, with `tele daemon` (#182).
var ErrUnsupported = errors.New("tele cannot be installed as a service on this system")

// ForThisSystem is the service manager of the system tele runs on, for the
// person whose home is home.
func ForThisSystem(home string) (Manager, error) {
	switch runtime.GOOS {
	case "darwin":
		return Launchd{Home: home, UID: os.Getuid(), Run: Run}, nil
	case "linux":
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		return Systemd{ConfigHome: config, Run: Run}, nil
	case "windows":
		return TaskScheduler{TempDir: os.TempDir(), Run: Run, Output: Output}, nil
	default:
		return nil, ErrUnsupported
	}
}

// wait is how an install or an uninstall waits for the old instance of a
// service to go, before it starts the new one or says it is done: a service
// manager may still hold the old one after it was told to stop it (#327).
type wait struct {
	// waiting, if set, is told that a wait has started; what it returns is
	// called once when it ends.
	waiting func() (done func())
	// sleep and now are the clock the wait runs by; the real one when nil.
	sleep func(time.Duration)
	now   func() time.Time
	// every is how often the wait looks.
	every time.Duration
}

// leaveLimit is the longest a wait lasts. launchd kills an agent that has not
// exited 20 seconds after it was told to.
const leaveLimit = 30 * time.Second

// ErrStillRunning is an old instance of a service that outlasted the wait for
// it, so that the new one could not be started; installing again, once it is
// gone, starts it.
var ErrStillRunning = fmt.Errorf("the running service did not stop in %s", leaveLimit)

// untilGone waits until gone says the old instance is gone, for at most
// leaveLimit, and reports whether it went.
func (w wait) untilGone(gone func() bool) bool {
	if gone() {
		return true
	}
	if w.waiting != nil {
		if done := w.waiting(); done != nil {
			defer done()
		}
	}
	sleep, now := w.sleep, w.now
	if sleep == nil {
		sleep = time.Sleep
	}
	if now == nil {
		now = time.Now
	}
	deadline := now().Add(leaveLimit)
	for now().Before(deadline) {
		sleep(w.every)
		if gone() {
			return true
		}
	}
	return false
}

// Launchd installs a unit as a LaunchAgent: the person's own, run in their
// session while they are logged in.
type Launchd struct {
	Home string
	UID  int
	Run  Runner
	// Waiting, Sleep and Now are as a wait's.
	Waiting func() (done func())
	Sleep   func(time.Duration)
	Now     func() time.Time
}

func (m Launchd) wait() wait {
	return wait{waiting: m.Waiting, sleep: m.Sleep, now: m.Now, every: 100 * time.Millisecond}
}

// loaded says whether launchd holds the label of the agent named name. It
// lets a label go only once the agent's process has exited, which takes up
// to its ExitTimeOut, and refuses to load one it still holds (#327).
func (m Launchd) loaded(name string) bool {
	return m.Run("launchctl", "print", m.domain()+"/"+Label(Unit{Name: name})) == nil
}

func (m Launchd) plist(name string) string {
	return filepath.Join(m.Home, "Library", "LaunchAgents", Label(Unit{Name: name})+".plist")
}

func (m Launchd) domain() string { return "gui/" + strconv.Itoa(m.UID) }

func (m Launchd) Install(u Unit) error {
	path := m.plist(u.Name)
	stayed := false
	if _, err := os.Stat(path); err == nil {
		// Unloaded first, or launchd keeps running what it loaded before. One
		// that was not loaded has nothing to unload. bootout's own error says
		// nothing: it fails on one still stopping as on one not loaded.
		_ = m.Run("launchctl", "bootout", m.domain()+"/"+Label(u))
		stayed = !m.wait().untilGone(func() bool { return !m.loaded(u.Name) })
	}
	if u.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(u.LogPath), 0o700); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, Plist(u), 0o644); err != nil {
		return err
	}
	// One that outlasted the wait may still have gone by now: tried once.
	err := m.Run("launchctl", "bootstrap", m.domain(), path)
	if err != nil && stayed {
		return fmt.Errorf("%w: %w", ErrStillRunning, err)
	}
	return err
}

func (m Launchd) Uninstall(name string) (bool, error) {
	path := m.plist(name)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	// An agent that is not loaded has nothing to stop; its file still goes.
	_ = m.Run("launchctl", "bootout", m.domain()+"/"+Label(Unit{Name: name}))
	gone := m.wait().untilGone(func() bool { return !m.loaded(name) })
	if err := os.Remove(path); err != nil {
		return false, err
	}
	if !gone {
		return true, ErrStillRunning
	}
	return true, nil
}

func (m Launchd) Installed(name string) (string, bool, error) {
	doc, err := os.ReadFile(m.plist(name))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	// The first string after the ProgramArguments key is the command.
	dec := xml.NewDecoder(bytes.NewReader(doc))
	afterKey := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", false, fmt.Errorf("%s names no command", m.plist(name))
		}
		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		var text string
		switch el.Name.Local {
		case "key":
			if err := dec.DecodeElement(&text, &el); err != nil {
				return "", false, err
			}
			afterKey = text == "ProgramArguments"
		case "string":
			if err := dec.DecodeElement(&text, &el); err != nil {
				return "", false, err
			}
			if afterKey {
				return text, true, nil
			}
		}
	}
}

// Systemd installs a unit as a systemd user unit, enabled for the person's
// login.
type Systemd struct {
	ConfigHome string
	Run        Runner
}

func (m Systemd) unit(name string) string {
	return filepath.Join(m.ConfigHome, "systemd", "user", name+".service")
}

func (m Systemd) Install(u Unit) error {
	path := m.unit(u.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, SystemdUnit(u), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", u.Name + ".service"},
		// Started if it was not running, started anew on the new unit if it
		// was.
		{"--user", "restart", u.Name + ".service"},
	} {
		if err := m.Run("systemctl", args...); err != nil {
			return err
		}
	}
	return nil
}

func (m Systemd) Uninstall(name string) (bool, error) {
	path := m.unit(name)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err := m.Run("systemctl", "--user", "disable", "--now", name+".service"); err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, m.Run("systemctl", "--user", "daemon-reload")
}

func (m Systemd) Installed(name string) (string, bool, error) {
	doc, err := os.ReadFile(m.unit(name))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	for line := range strings.Lines(string(doc)) {
		if cmd, ok := strings.CutPrefix(strings.TrimSpace(line), "ExecStart="); ok {
			return systemdUnquote(cmd), true, nil
		}
	}
	return "", false, fmt.Errorf("%s names no command", m.unit(name))
}

// systemdUnquote reads the first argument of an ExecStart that systemdQuote
// wrote.
func systemdUnquote(cmd string) string {
	if !strings.HasPrefix(cmd, `"`) {
		first, _, _ := strings.Cut(cmd, " ")
		return first
	}
	var b strings.Builder
	for i := 1; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '\\' && i+1 < len(cmd):
			i++
			b.WriteByte(cmd[i])
		case c == '"':
			return b.String()
		case (c == '%' || c == '$') && i+1 < len(cmd) && cmd[i+1] == c:
			i++
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// TaskScheduler installs a unit as a Task Scheduler task started at logon: a
// Windows service would run where no notification can be shown (ADR 0030).
type TaskScheduler struct {
	// TempDir is where the task's XML is written for schtasks to read.
	TempDir string
	Run     Runner
	// Output runs a command for what it prints.
	Output func(name string, args ...string) ([]byte, error)
}

func (m TaskScheduler) Installed(name string) (string, bool, error) {
	printed, err := m.Output("schtasks", "/Query", "/TN", name, "/XML")
	if err != nil {
		return "", false, nil
	}
	var task struct {
		Command string `xml:"Actions>Exec>Command"`
	}
	dec := xml.NewDecoder(strings.NewReader(fromUTF16(printed)))
	// Whatever the declaration says, the text is UTF-8 by now.
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	if err := dec.Decode(&task); err != nil {
		return "", false, fmt.Errorf("reading the task %s: %w", name, err)
	}
	return task.Command, true, nil
}

// fromUTF16 is b as text: UTF-16 when it starts with the little-endian byte
// order mark, which is how schtasks may print, and UTF-8 otherwise.
func fromUTF16(b []byte) string {
	if len(b) < 2 || b[0] != 0xFF || b[1] != 0xFE {
		return string(b)
	}
	units := make([]uint16, 0, len(b)/2)
	for i := 2; i+1 < len(b); i += 2 {
		units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(units))
}

func (m TaskScheduler) Install(u Unit) error {
	f, err := os.CreateTemp(m.TempDir, "tele-task-*.xml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(TaskXML(u)); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Ended first, so the task starts on the command it is given now. One not
	// running has nothing to end.
	_ = m.Run("schtasks", "/End", "/TN", u.Name)
	if err := m.Run("schtasks", "/Create", "/TN", u.Name, "/XML", f.Name(), "/F"); err != nil {
		return err
	}
	return m.Run("schtasks", "/Run", "/TN", u.Name)
}

func (m TaskScheduler) Uninstall(name string) (bool, error) {
	if err := m.Run("schtasks", "/Query", "/TN", name); err != nil {
		return false, nil
	}
	_ = m.Run("schtasks", "/End", "/TN", name)
	if err := m.Run("schtasks", "/Delete", "/TN", name, "/F"); err != nil {
		return false, err
	}
	return true, nil
}
