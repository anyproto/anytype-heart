//go:build windows

package udpprobe

import (
	"errors"
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sioUDPConnReset = _WSAIOW(IOC_VENDOR, 12). Winsock enables this behavior by
// default (ICMP port-unreachable completes a pending recv with
// WSAECONNRESET), but the Go runtime turns it off on every UDP socket
// (golang.org/issue/5834), which would blind this probe.
const sioUDPConnReset = uint32(windows.IOC_IN | windows.IOC_VENDOR | 12)

func enableRefusedReports(conn *net.UDPConn) error {
	rc, err := conn.SyscallConn()
	if err != nil {
		return fmt.Errorf("get raw conn: %w", err)
	}
	var ctrlErr error
	if err = rc.Control(func(fd uintptr) {
		enabled := uint32(1)
		var ret uint32
		ctrlErr = windows.WSAIoctl(windows.Handle(fd), sioUDPConnReset,
			(*byte)(unsafe.Pointer(&enabled)), uint32(unsafe.Sizeof(enabled)),
			nil, 0, &ret, nil, 0)
	}); err != nil {
		return fmt.Errorf("raw conn control: %w", err)
	}
	if ctrlErr != nil {
		return fmt.Errorf("enable SIO_UDP_CONNRESET: %w", ctrlErr)
	}
	return nil
}

func isRefused(err error) bool {
	return errors.Is(err, windows.WSAECONNRESET) || errors.Is(err, windows.WSAECONNREFUSED)
}
