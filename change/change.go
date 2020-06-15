package change

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type Change struct {
	Id          string
	Next        []*Change
	detailsOnly bool
	*pb.Change
}

func (ch *Change) GetLastSnapshotId() string {
	if ch.Snapshot != nil {
		return ch.Id
	}
	return ch.LastSnapshotId
}

func (ch *Change) HasDetails() bool {
	if ch.Snapshot != nil {
		return true
	}
	for _, ct := range ch.Content {
		if ct.GetDetailsSet() != nil {
			return true
		}
		if ct.GetDetailsUnset() != nil {
			return true
		}
	}
	return false
}
