package filesync

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
)

type QueueItem struct {
	SpaceId     string
	ObjectId    string
	FileId      domain.FileId
	Timestamp   float64
	AddedByUser bool
	Imported    bool

	Variants []domain.FileId
}

func (it *QueueItem) Validate() error {
	if it.ObjectId == "" {
		return fmt.Errorf("empty object id")
	}
	if !it.FileId.Valid() {
		return fmt.Errorf("invalid file id")
	}
	return nil
}

func (it *QueueItem) Key() string {
	return it.ObjectId
}

func (it *QueueItem) FullFileId() domain.FullFileId {
	return domain.FullFileId{
		SpaceId: it.SpaceId,
		FileId:  it.FileId,
	}
}

func queueItemLess(one, other *QueueItem) bool {
	return one.Timestamp < other.Timestamp
}
