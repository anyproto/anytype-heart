package filesync

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
)

func (s *fileSync) process(id string, proc func(exists bool, info FileInfo) (FileInfo, error)) error {
	item, err := s.queue.GetById(id)
	if err != nil && !errors.Is(err, filequeue.ErrNotFound) {
		return fmt.Errorf("get item: %w", err)
	}
	exists := !errors.Is(err, filequeue.ErrNotFound)

	next, err := proc(exists, item)
	if err != nil {
		return errors.Join(s.queue.ReleaseAndUpdate(item), fmt.Errorf("process item: %w", err))
	}
	return s.queue.ReleaseAndUpdate(next)
}

func filterByFileId(fileId string) query.Key {
	return query.Key{
		Path:   []string{"fileId"},
		Filter: query.NewComp(query.CompOpEq, fileId),
	}
}

func filterByState(state FileState) query.Key {
	return query.Key{
		Path:   []string{"state"},
		Filter: query.NewComp(query.CompOpEq, int(state)),
	}
}

func filterBySpaceId(spaceId string) query.Key {
	return query.Key{
		Path:   []string{"spaceId"},
		Filter: query.NewComp(query.CompOpEq, spaceId),
	}
}

func filterByBytesToUpload(toUpload int) query.Key {
	return query.Key{
		Path:   []string{"bytesToUploadOrBind"},
		Filter: query.NewComp(query.CompOpLte, toUpload),
	}
}

func orderByScheduledAt() *query.SortField {
	return &query.SortField{
		Field:   "scheduledAt",
		Path:    []string{"scheduledAt"},
		Reverse: false,
	}
}
