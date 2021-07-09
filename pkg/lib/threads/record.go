package threads

import (
	"github.com/libp2p/go-libp2p-core/peer"
	threadsNet "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
)

type threadRecord struct {
	threadsNet.Record
	threadID thread.ID
	logID    peer.ID
}

func (t threadRecord) Value() threadsNet.Record {
	return t.Record
}

func (t threadRecord) ThreadID() thread.ID {
	return t.threadID
}

func (t threadRecord) LogID() peer.ID {
	return t.logID
}
