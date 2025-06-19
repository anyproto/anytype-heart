package netutil

import "net"

func GetRandomPort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}

	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// GetAvailableAddr returns an available address based on the preferred address.
// if port on the preferred address is not available, it will return a random port.
// there is a possibility that the returned address will be taken by another process in the meantime,
// so it's better to use GetListener
// preferredAddr defaults to localhost:0
func GetAvailableAddr(preferredAddr string) (string, error) {
	l, err := GetTcpListener(preferredAddr)
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).String(), nil
}

// GetTcpListener returns a TCP net.Listener on a preferred address.
// if specified port is not available, it will try to Listen on a random port.
// if it's still not possible to get the random port on the preferred address IP, it will try to get a listener on localhost.
// if all fails, it will return an error.
// preferredAddr defaults to localhost:0
func GetTcpListener(preferredAddr string) (net.Listener, error) {
	if preferredAddr == "" {
		preferredAddr = "localhost:0"
	}

	addr, err := net.ResolveTCPAddr("tcp", preferredAddr)
	if err != nil {
		return nil, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		addr.Port = 0
		// reset the port to 0 to get a random port
		l, err = net.ListenTCP("tcp", addr)
		if err != nil {
			if !addr.IP.IsLoopback() {
				// reset the addr to localhost
				addr.IP = net.IPv4(127, 0, 0, 1)
				l, err = net.ListenTCP("tcp", addr)
				if err != nil {
					return nil, err
				}
				return l, nil
			}
			return nil, err
		}
	}

	return l, nil
}
