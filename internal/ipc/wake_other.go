//go:build !windows

package ipc

import "time"

// wakeEvery is zero here: a waiting read wakes when the peer closes.
const wakeEvery time.Duration = 0
