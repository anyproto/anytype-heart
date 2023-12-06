package objectcreator

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

func (s *service) updateSystemObjects(spaceId string, objects map[string]*types.Struct) {
	marketRels, err := s.objectStore.ListAllRelations(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		log.Errorf("failed to get relations from marketplace space: %v", err)
		return
	}

	marketTypes, err := s.listAllObjectTypes(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		log.Errorf("failed to get object types from marketplace space: %v", err)
		return
	}

	spaceIds, err := s.storage.AllSpaceIds()
	if err != nil {
		log.Errorf("failed to get spaces ids from the storage: %v", err)
		return
	}
}
