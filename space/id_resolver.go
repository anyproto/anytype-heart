package space

import (
	"fmt"
	"github.com/anyproto/any-sync/commonspace"
)

func (s *service) ResolveSpaceID(objectID string) (string, error) {
	return s.objectStore.ResolveSpaceID(objectID)
}

func (s *service) StoreSpaceID(objectID string, spaceID string) error {
	fmt.Println("store", objectID, spaceID)
	return s.objectStore.StoreSpaceID(objectID, spaceID)
}

func (s *service) storeMappingForSpace(spc commonspace.Space) error {
	for _, id := range spc.StoredIds() {
		if err := s.objectStore.StoreSpaceID(id, spc.Id()); err != nil {
			return err
		}
	}
	return nil
}
