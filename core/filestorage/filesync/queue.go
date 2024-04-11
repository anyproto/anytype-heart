package filesync

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
)

var (
	uploadingKeyPrefix      = []byte(keyPrefix + "queue/uploading/")
	deletionKeyPrefix       = []byte(keyPrefix + "queue/deletion/")
	retryUploadingKeyPrefix = []byte(keyPrefix + "queue/retry-uploading/")
	retryDeletionKeyPrefix  = []byte(keyPrefix + "queue/retry-deletion/")
)

type QueueItem struct {
	SpaceId     string
	ObjectId    string
	FileId      domain.FileId
	Timestamp   int64
	AddedByUser bool
	Imported    bool
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

func (it *QueueItem) Less(other *QueueItem) bool {
	return it.Timestamp < other.Timestamp
}
