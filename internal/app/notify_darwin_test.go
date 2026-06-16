//go:build darwin

package app

import (
	"os"
	"testing"
)

// TestDarwinNotifierManual exercises the real applet build + post path. It is
// gated behind TELE_NOTIFY_MANUAL=1 because it shells out to osacompile and
// raises a visible desktop notification, which is not appropriate for CI.
//
//	TELE_NOTIFY_MANUAL=1 go test ./internal/app -run TestDarwinNotifierManual -v
func TestDarwinNotifierManual(t *testing.T) {
	if os.Getenv("TELE_NOTIFY_MANUAL") != "1" {
		t.Skip("set TELE_NOTIFY_MANUAL=1 to run the manual notification test")
	}
	n, err := newDarwinNotifier()
	if err != nil {
		t.Fatalf("newDarwinNotifier: %v", err)
	}
	if err := n.Notify("Tele test", "Из Go-кода — кликни, поднимется терминал"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	t.Logf("posted; target=%q", os.Getenv("__CFBundleIdentifier"))
}
