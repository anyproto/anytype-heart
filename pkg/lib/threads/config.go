package threads

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"

	cafePb "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
)

const (
	defaultCafeNodeP2P = "/ip4/127.0.0.1/tcp/5001/p2p/12D3KooWRNcPvzFigfUZuqvak7vfQGCcvqotjE7R2KBTfnkKyVvj"
)

type CafeConfigFetcher interface {
	GetCafeConfig() *cafePb.GetConfigResponseConfig
}

type CurrentWorkspaceThreadGetter interface {
	GetCurrentWorkspaceId() (string, error)
}

type ThreadCreateQueue interface {
	AddThreadQueueEntry(entry *model.ThreadCreateQueueEntry) (err error)
	RemoveThreadQueueEntry(threadId string) (err error)
	GetAllQueueEntries() ([]*model.ThreadCreateQueueEntry, error)
}

type ObjectDeleter interface {
	DeleteObject(id string) error
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
