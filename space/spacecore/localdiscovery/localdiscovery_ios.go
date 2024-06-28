package localdiscovery

import (
	"fmt"
	gonet "net"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/net/addrs"
)

func isP2PPossible(ifaces addrs.InterfacesAddrs) bool {
	for _, iface := range ifaces.Interfaces {
		// only check ethernet interface
		if !strings.HasPrefix(iface.Name, "en") {
			continue
		}

		// we want to check whatever we have iOS local network permission enabled for the app
		addrs, err := iface.Addrs()
		if err != nil {
			log.Error("Local discovery: Error getting addresses", zap.Error(err))
			return false
		}

		for _, addr := range addrs {
			ipv4, _ := parseAddr(addr)
			if ipv4 != "" {
				if err = tryToConnectOurselves(ipv4, 3*time.Second); err == nil {
					return true
				} else {
					log.Debug("Local discovery: Error connecting", zap.String("addr", addr.String()), zap.Error(err))
				}
			}
		}
	}
	log.Info("Local discovery is not possible")
	return false
}

// tryToConnectOurselves starts a listener on the given address and tries to connect to it.
// this trick allows us to check whether we have local network permission on iOS enabled for the app
func tryToConnectOurselves(addr string, timeout time.Duration) (err error) {
	listener, err := gonet.Listen("tcp", addr+":0")
	if err != nil {
		return err
	}
	defer listener.Close()
	_, port, err := gonet.SplitHostPort(listener.Addr().String())
	if err != nil {
		return err
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {

				// check if connection is closed
				log.Debug("Error accepting", zap.Error(err))
				break
			}
			_ = conn.Close()
		}
	}()

	conn, err := gonet.DialTimeout("tcp", fmt.Sprintf("%s:%s", addr, port), timeout)
	if err != nil {
		return
	}
	_ = conn.Close()
	_ = listener.Close()
	return
}
