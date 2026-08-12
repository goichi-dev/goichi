//go:build !windows

package goichi

import (
	"golang.org/x/sys/unix"
	"net"
	"syscall"
)

func getListenConfig(multiprocess bool) net.ListenConfig {
	if multiprocess {
		return net.ListenConfig{
			Control: func(network, address string, c syscall.RawConn) error {
				return c.Control(func(fd uintptr) {
					unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
				})
			},
		}
	}
	return net.ListenConfig{}
}

func startMonitor(children map[int]string, restartChild func(pid int)) {
	go func() {
		for {
			pid, err := syscall.Wait4(-1, nil, 0, nil)
			if err != nil {
				continue
			}

			if _, ok := children[pid]; ok {
				delete(children, pid)
				restartChild(pid)
			}
		}
	}()
}
