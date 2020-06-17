package change

import (
	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func NewChangeFromRecord(detailsOnly bool, record core.SmartblockRecordWithLogID) (*Change, error) {
	var ch = &pb.Change{}
	if err := record.Unmarshal(ch); err != nil {
		return nil, err
	}
	return &Change{
		Id:          record.ID,
		Change:      ch,
		detailsOnly: detailsOnly,
	}, nil
}

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
