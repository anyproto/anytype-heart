package netutil

import (
	"net"
	"testing"
	"time"
)

// Test that an empty preferredAddr defaults to "localhost:0" and returns a valid listener.
func TestGetTcpListenerDefault(t *testing.T) {
	l, err := GetTcpListener("")
	if err != nil {
		t.Fatalf("expected no error when using default address, got: %v", err)
	}
	defer l.Close()

	addr := l.Addr().String()
	if addr == "" {
		t.Fatalf("expected a valid listener address, got empty")
	}
	t.Logf("Default listener address: %s", addr)
}

// Test that if a specific port is in use, the function falls back to a random port.
func TestGetTcpListenerPortInUse(t *testing.T) {
	preferredAddr := "127.0.0.1:0"

	// Occupy the port.
	l1, err := net.Listen("tcp", preferredAddr)
	if err != nil {
		t.Fatalf("failed to occupy port %s: %v", preferredAddr, err)
	}
	defer l1.Close()

	// Now call GetTcpListener with the same address.
	l2, err := GetTcpListener(l1.Addr().String())
	if err != nil {
		t.Fatalf("expected GetTcpListener to succeed when preferred port is in use, got error: %v", err)
	}
	defer l2.Close()

	tcpAddr, ok := l2.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address is not a TCPAddr")
	}
	if tcpAddr.Port == l1.Addr().(*net.TCPAddr).Port {
		t.Fatalf("expected a fallback to a random port, but got the same port %d", tcpAddr.Port)
	}
	t.Logf("Fallback listener address: %s", tcpAddr.String())
}

// Test that an invalid address returns an error.
func TestGetTcpListenerInvalidAddr(t *testing.T) {
	_, err := GetTcpListener("invalid:address")
	if err == nil {
		t.Fatalf("expected an error for an invalid address, got nil")
	}
	t.Logf("Invalid address error: %v", err)
}

// Test that if the preferred address is non-loopback and binding fails,
// the function falls back to binding on localhost.
func TestGetTcpListenerNonLoopbackFailover(t *testing.T) {
	// Use an IP that is almost certainly not assigned to a local interface.
	// "192.0.2.1" is reserved for documentation.
	preferredAddr := "192.0.2.1:8080"

	l, err := GetTcpListener(preferredAddr)
	if err != nil {
		t.Fatalf("expected failover to succeed when binding to non-loopback fails, got error: %v", err)
	}
	defer l.Close()

	tcpAddr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address is not a TCPAddr")
	}
	// The final IP should be the loopback address.
	if !tcpAddr.IP.IsLoopback() {
		t.Fatalf("expected failover to localhost, but got IP: %s", tcpAddr.IP.String())
	}
	t.Logf("Non-loopback failover listener address: %s", tcpAddr.String())
}

// Test that when using a random port specification (":0") a non-zero port is chosen.
func TestGetTcpListenerRandomPort(t *testing.T) {
	l, err := GetTcpListener("127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected no error when using a random port, got: %v", err)
	}
	defer l.Close()

	tcpAddr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address is not a TCPAddr")
	}
	if tcpAddr.Port == 0 {
		t.Fatalf("expected a random non-zero port, got port 0")
	}
	t.Logf("Random port listener address: %s", tcpAddr.String())
}

// Optionally, a helper to close listeners gracefully after a short delay
func closeListener(l net.Listener) {
	// Ensure the listener is closed (if needed, sometimes a small delay helps avoid race conditions)
	_ = l.Close()
	time.Sleep(10 * time.Millisecond)
}
