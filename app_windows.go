//go:build windows

package goichi

import (
	"net"
)

func getListenConfig(_ bool) net.ListenConfig {
	// Windows does not support SO_REUSEPORT in the same way,
	// returning empty ListenConfig for standard behavior
	return net.ListenConfig{}
}

func startMonitor(_ map[int]string, _ func(pid int)) {
	// No-op on Windows
}
