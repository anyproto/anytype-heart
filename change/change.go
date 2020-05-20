package change

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type Change struct {
	Id   string
	Next []*Change
	*pb.Change
}

func (ch *Change) GetLastSnapshotId() string {
	if ch.Snapshot != nil {
		return ch.Id
	}
	return ch.LastSnapshotId
}
