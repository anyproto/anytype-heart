package filesync

import (
	"github.com/anyproto/anytype-heart/core/domain"
)

var (
	uploadKeyPrefix    = []byte(keyPrefix + "queue/upload/")
	removeKeyPrefix    = []byte(keyPrefix + "queue/remove/")
	discardedKeyPrefix = []byte(keyPrefix + "queue/discarded/")
)

type QueueItem struct {
	SpaceId     string
	FileId      domain.FileId
	Timestamp   int64
	AddedByUser bool
	Imported    bool
}

func (it *QueueItem) Key() string {
	return it.SpaceId + "/" + it.FileId.String()
}

func (it *QueueItem) FullFileId() domain.FullFileId {
	return domain.FullFileId{
		SpaceId: it.SpaceId,
		FileId:  it.FileId,
	}
}

func (it *QueueItem) Less(other *QueueItem) bool {
	return it.Timestamp < other.Timestamp
}
