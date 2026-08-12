//go:build windows

package goichi

import (
	"net"
)

// multiprocessSupported is false because Windows has no SO_REUSEPORT: every
// child would try to bind the same port and all but one would fail.
const multiprocessSupported = false

func getListenConfig(_ bool) net.ListenConfig {
	// Windows does not support SO_REUSEPORT in the same way,
	// returning empty ListenConfig for standard behavior
	return net.ListenConfig{}
}

func startMonitor(_ map[int]string, _ func(pid int)) {
	// No-op on Windows
}
