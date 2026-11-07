//go:build windows

package ipc

import "time"

// wakeEvery is how long a read waits before it is started again. It is how
// late a peer's going is seen in the rare read Windows does not wake.
const wakeEvery = time.Second
