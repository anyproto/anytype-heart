package offlinenet

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/transport"
)

const (
	QuicCName  = "net.transport.quic"
	YamuxCName = "net.transport.yamux"
)

var errOffline = errors.New("network transport disabled in offline mode")

// offlineTransport is an in-process no-op transport. It satisfies the
// any-sync transport.Transport interface so that peerservice keeps working,
// but it never dials and never accepts connections. This removes quic-go /
// go-libp2p-yamux from the dependency graph of the mobile entry.
type offlineTransport struct {
	name     string
	accepter transport.Accepter
}

func (t *offlineTransport) Init(*app.App) (err error) { return nil }

func (t *offlineTransport) Name() string { return t.name }

func (t *offlineTransport) Run(context.Context) error { return nil }

func (t *offlineTransport) Close(context.Context) error { return nil }

func (t *offlineTransport) SetAccepter(accepter transport.Accepter) {
	t.accepter = accepter
}

func (t *offlineTransport) Dial(ctx context.Context, addr string) (transport.MultiConn, error) {
	return nil, errOffline
}

func NewQuic() app.Component  { return &offlineTransport{name: QuicCName} }
func NewYamux() app.Component { return &offlineTransport{name: YamuxCName} }
