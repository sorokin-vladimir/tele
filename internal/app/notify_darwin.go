//go:build darwin

package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// On macOS beeep posts notifications via `osascript`, so the system attributes
// them to Script Editor and clicking one launches Script Editor (#17). To make
// a notification own its identity and route clicks back to the terminal, we
// post through a tiny AppleScript applet that we generate on first run:
//
//   - the notification belongs to the applet's bundle, so it shows as "Tele";
//   - clicking it relaunches the applet, which raises the terminal that started
//     us (resolved at startup from $__CFBundleIdentifier — works for any
//     terminal, not just one hardcoded app).
//
// osacompile/PlistBuddy/codesign/lsregister are all part of macOS, so this
// stays dependency-free and CGo-free. If any step fails we fall back to beeep.

// appletBundleID is stable so the user's notification permission survives
// rebuilds (macOS keys authorization by bundle id, not by name on disk).
const appletBundleID = "com.sorokin.tele.notifier"

// appletVersion is bumped whenever appletSource changes so an installed applet
// is rebuilt instead of silently running stale AppleScript.
const appletVersion = "1"

// unitSep separates title from body inside a queue entry. It cannot appear in
// human text, so the applet can split on it without escaping concerns.
const unitSep = "\x1f"

// appletSource is the AppleScript the applet runs. Posting writes one file per
// notification into queue/ and launches the applet; the applet drains every
// queue entry it finds and posts them. A launch that finds an empty queue is a
// click, so it raises the terminal instead. Draining in both `run` and `reopen`
// keeps bursts (where a second launch is coalesced into the live instance)
// from being misread as clicks.
const appletSource = `property supDir : (POSIX path of (path to home folder)) & "Library/Application Support/tele/"
property queueDir : supDir & "queue/"

on readFile(p)
	try
		return (do shell script "cat " & quoted form of p)
	on error
		return ""
	end try
end readFile

on raiseTerminal()
	set tgt to my readFile(supDir & "target")
	if tgt is not "" then
		try
			do shell script "open -b " & quoted form of tgt
		end try
	end if
end raiseTerminal

on handleLaunch()
	set hadEntry to false
	set theFiles to {}
	try
		set theFiles to paragraphs of (do shell script "ls -1 " & quoted form of queueDir & " 2>/dev/null || true")
	end try
	repeat with f in theFiles
		set fn to f as string
		if fn is not "" then
			set hadEntry to true
			set fp to queueDir & fn
			set raw to my readFile(fp)
			do shell script "rm -f " & quoted form of fp
			set AppleScript's text item delimiters to (ASCII character 31)
			set parts to text items of raw
			set AppleScript's text item delimiters to ""
			if (count of parts) is greater than or equal to 2 then
				display notification (item 2 of parts) with title (item 1 of parts)
			else if raw is not "" then
				display notification raw with title "Tele"
			end if
		end if
	end repeat
	if not hadEntry then my raiseTerminal()
end handleLaunch

on run
	my handleLaunch()
end run

on reopen
	my handleLaunch()
end reopen
`

type darwinNotifier struct {
	appPath  string
	queueDir string
	seq      atomic.Uint64
}

// newNotifier builds (or reuses) the applet and returns a notifier backed by
// it. Any setup failure degrades gracefully to the beeep fallback.
func newNotifier(log *zap.Logger) Notifier {
	n, err := newDarwinNotifier()
	if err != nil {
		log.Warn("notifications: applet setup failed, using beeep fallback", zap.Error(err))
		return beeepNotifier{}
	}
	return n
}

func newDarwinNotifier() (*darwinNotifier, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	supDir := filepath.Join(home, "Library", "Application Support", "tele")
	queueDir := filepath.Join(supDir, "queue")
	appPath := filepath.Join(supDir, "Tele.app")

	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		return nil, fmt.Errorf("create queue dir: %w", err)
	}
	// Record which terminal launched us so a click can raise it. Empty when not
	// started from a GUI app (e.g. ssh) — the applet then no-ops on click.
	if err := os.WriteFile(filepath.Join(supDir, "target"), []byte(os.Getenv("__CFBundleIdentifier")), 0o644); err != nil {
		return nil, fmt.Errorf("write target: %w", err)
	}
	if err := ensureApplet(supDir, appPath); err != nil {
		return nil, err
	}
	return &darwinNotifier{appPath: appPath, queueDir: queueDir}, nil
}

// ensureApplet builds the applet unless an up-to-date one already exists.
func ensureApplet(supDir, appPath string) error {
	versionFile := filepath.Join(supDir, "applet.version")
	if _, err := os.Stat(appPath); err == nil {
		if v, err := os.ReadFile(versionFile); err == nil && string(v) == appletVersion {
			return nil
		}
	}

	srcFile := filepath.Join(supDir, "applet.applescript")
	if err := os.WriteFile(srcFile, []byte(appletSource), 0o644); err != nil {
		return fmt.Errorf("write applet source: %w", err)
	}
	defer os.Remove(srcFile) //nolint:errcheck

	// osacompile refuses to overwrite an existing bundle, so clear it first.
	if err := os.RemoveAll(appPath); err != nil {
		return fmt.Errorf("remove old applet: %w", err)
	}
	if err := run("osacompile", "-o", appPath, srcFile); err != nil {
		return fmt.Errorf("osacompile: %w", err)
	}

	plist := filepath.Join(appPath, "Contents", "Info.plist")
	// Give the applet a stable bundle id (for permission + click ownership) and
	// a friendly name. The applet from osacompile has neither.
	if err := run("/usr/libexec/PlistBuddy", "-c", "Add :CFBundleIdentifier string "+appletBundleID, plist); err != nil {
		return fmt.Errorf("set bundle id: %w", err)
	}
	if err := run("/usr/libexec/PlistBuddy", "-c", "Add :CFBundleDisplayName string Tele", plist); err != nil {
		return fmt.Errorf("set display name: %w", err)
	}
	// Ad-hoc sign so LaunchServices trusts the patched bundle, then register it.
	if err := run("codesign", "--force", "--deep", "-s", "-", appPath); err != nil {
		return fmt.Errorf("codesign: %w", err)
	}
	if err := run(lsregisterPath, "-f", appPath); err != nil {
		return fmt.Errorf("lsregister: %w", err)
	}

	if err := os.WriteFile(versionFile, []byte(appletVersion), 0o644); err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	return nil
}

const lsregisterPath = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, out)
	}
	return nil
}

func (d *darwinNotifier) Notify(title, body string) error {
	// Atomically drop a payload into the queue (write to a temp name, then
	// rename) so the applet never reads a half-written entry.
	name := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatUint(d.seq.Add(1), 10)
	tmp := filepath.Join(d.queueDir, "."+name+".tmp")
	final := filepath.Join(d.queueDir, name)
	if err := os.WriteFile(tmp, []byte(title+unitSep+body), 0o644); err != nil {
		return fmt.Errorf("write queue entry: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp) //nolint:errcheck
		return fmt.Errorf("commit queue entry: %w", err)
	}
	// -g keeps the applet in the background so posting never steals focus.
	return run("open", "-g", d.appPath)
}
