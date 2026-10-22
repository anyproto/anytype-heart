package clientserver

import "net"

// probeSocket is not needed on Windows: the listener lifecycle runs on iOS only.
func probeSocket(net.Listener) error {
	return nil
}
