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

	// VariantId tells uploader to upload specific branch of file tree
	VariantId domain.FileId
	// Score affects priority
	Score int
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
	if it.VariantId != "" {
		return it.ObjectId + "/" + it.VariantId.String()
	}
	return it.ObjectId
}

func (it *QueueItem) FullFileId() domain.FullFileId {
	return domain.FullFileId{
		SpaceId: it.SpaceId,
		FileId:  it.FileId,
	}
}

func queueItemLess(one, other *QueueItem) bool {
	if one.Score != other.Score {
		return one.Score > other.Score
	}
	return one.Timestamp < other.Timestamp
}
