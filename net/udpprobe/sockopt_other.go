//go:build !windows

package udpprobe

import (
	"errors"
	"net"
	"syscall"
)

// enableRefusedReports is a no-op outside Windows: BSD-socket platforms
// already surface ICMP port-unreachable as ECONNREFUSED on connected sockets.
func enableRefusedReports(_ *net.UDPConn) error {
	return nil
}

func isRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}
