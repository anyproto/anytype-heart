//go:build !windows

package clientserver

import (
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// getsockoptInt is a seam: a test injects a socket error no real listener
// can be made to carry.
var getsockoptInt = defaultGetsockoptInt

var defaultGetsockoptInt = unix.GetsockoptInt

// probeSocket reads the listening socket's pending error. iOS sets EBADF on a
// socket it defuncted (XNU sodefunct); a healthy listener reports 0. SO_ERROR
// is read-and-clear, so a caller must act on the first failure.
func probeSocket(lis net.Listener) error {
	sc, ok := lis.(syscall.Conn)
	if !ok {
		return nil
	}
	rc, err := sc.SyscallConn()
	if err != nil {
		return fmt.Errorf("syscall conn: %w", err)
	}
	var soErr int
	var getErr error
	if err = rc.Control(func(fd uintptr) {
		soErr, getErr = getsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_ERROR)
	}); err != nil {
		return fmt.Errorf("control: %w", err)
	}
	if getErr != nil {
		return fmt.Errorf("getsockopt SO_ERROR: %w", getErr)
	}
	if soErr != 0 {
		return fmt.Errorf("socket error: %w", syscall.Errno(soErr))
	}
	return nil
}
