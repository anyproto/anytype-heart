package threads

import cafePb "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"

const (
	defaultCafeNodeP2P = "/dns4/cafe1.anytype.io/tcp/4001/p2p/12D3KooWKwPC165PptjnzYzGrEs7NSjsF5vvMmxmuqpA2VfaBbLw"
)

type CafeConfigFetcher interface {
	FetchCafeConfig(untilSuccess bool) (cfg *cafePb.GetConfigResponseConfig, err error)
}

type Config struct {
	SyncTracking bool
	Debug        bool
	PubSub       bool
	Metrics      bool

	CafeP2PAddr             string
	CafePermanentConnection bool // explicitly watch the connection to this peer and reconnect in case the connection has failed
}

type ThreadsConfigGetter interface {
	ThreadsConfig() Config
}

var DefaultConfig = Config{
	SyncTracking:            true,
	Debug:                   false,
	Metrics:                 false,
	PubSub:                  true,
	CafeP2PAddr:             defaultCafeNodeP2P,
	CafePermanentConnection: true,
}
