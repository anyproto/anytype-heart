package filesync

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
)

func (s *fileSync) process(id string, proc func(exists bool, info FileInfo) (FileInfo, bool, error)) error {
	item, err := s.queue.GetById(id)
	if err != nil && !errors.Is(err, filequeue.ErrNotFound) {
		return fmt.Errorf("get item: %w", err)
	}
	exists := !errors.Is(err, filequeue.ErrNotFound)

	release := func(id string, update bool, info FileInfo) error {
		if update {
			return s.queue.ReleaseAndUpdate(id, info)
		} else {
			return s.queue.Release(id)
		}
	}

	next, update, err := proc(exists, item)
	if err != nil {
		return errors.Join(release(id, update, next), fmt.Errorf("process item: %w", err))
	}
	return release(id, update, next)
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
