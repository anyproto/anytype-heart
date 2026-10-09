package localdiscovery

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_testSelfConnection(t *testing.T) {
	err := testSelfConnection("127.0.0.1")
	require.NoError(t, err)

	err = testSelfConnection("11.11.11.11")
	require.Error(t, err)
}

func Test_testSelfConnectionZoned(t *testing.T) {
	// a zoned link-local address, as on a cable without IPv4
	lo := loopbackLinkLocal(t)
	require.NoError(t, testSelfConnection(lo))
}

func loopbackLinkLocal(t *testing.T) string {
	t.Helper()
	ifaces, err := net.Interfaces()
	require.NoError(t, err)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		require.NoError(t, err)
		for _, a := range addrs {
			if ipNet, ok := a.(*net.IPNet); ok && ipNet.IP.To4() == nil && ipNet.IP.IsLinkLocalUnicast() {
				return ipNet.IP.String() + "%" + iface.Name
			}
		}
	}
	t.Skip("no link-local address on loopback")
	return ""
}
