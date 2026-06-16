//go:build !darwin

package app

import "go.uber.org/zap"

// newNotifier returns the desktop notifier for non-macOS platforms. beeep is
// the cross-platform fallback; macOS uses a richer applet-based notifier (see
// notify_darwin.go) so that clicking a notification raises the source terminal.
func newNotifier(_ *zap.Logger) Notifier {
	return beeepNotifier{}
}
