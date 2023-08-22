package editor

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	dataview2 "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func migrateSourcesInDataview(sb smartblock.SmartBlock, s *state.State, relationService relation.Service) {
	s.Iterate(func(block simple.Block) bool {
		if block.Model().GetDataview() != nil {
			// Mark block as mutable
			dv := s.Get(block.Model().Id).(dataview2.Block)
			newSources := make([]string, 0, len(dv.GetSource()))
			for _, src := range dv.GetSource() {
				typeKey, err := bundle.TypeKeyFromUrl(src)
				if err == nil {
					typeID, err := relationService.GetTypeIdByKey(context.Background(), sb.SpaceID(), typeKey)
					if err != nil {
						log.Errorf("migrate dataview sources: failed to get type id by key: %v", err)
						continue
					}
					newSources = append(newSources, typeID)
					continue
				}

				relationKey, err := bundle.RelationKeyFromID(src)
				if err == nil {
					relationID, err := relationService.GetRelationIdByKey(context.Background(), sb.SpaceID(), relationKey)
					if err != nil {
						log.Errorf("migrate dataview sources: failed to get relation id by key: %v", err)
						continue
					}
					newSources = append(newSources, relationID)
					continue
				}

				newSources = append(newSources, src)
			}
			dv.SetSource(newSources)
		}
		return true
	})
}
