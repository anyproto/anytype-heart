package smartblock

import (
	"context"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (sb *smartBlock) updateBackLinks(s *state.State) {
	backLinks, err := sb.objectStore.GetInboundLinksByID(sb.Id())
	if err != nil {
		log.With("objectID", sb.Id()).Errorf("failed to get inbound links from object store: %s", err)
		return
	}
	s.SetDetailAndBundledRelation(bundle.RelationKeyBacklinks, pbtypes.StringList(backLinks))
}

func (sb *smartBlock) injectLinksDetails(s *state.State) {
	links := sb.navigationalLinks(s)
	links = slice.RemoveMut(links, sb.Id())
	s.SetLocalDetail(bundle.RelationKeyLinks, pbtypes.StringList(links))
}

func (sb *smartBlock) navigationalLinks(s *state.State) (ids []string) {
	if !internalflag.NewFromState(s).Has(model.InternalFlag_collectionDontIndexLinks) {
		// flag used when importing a large set of objects
		ids = append(ids, s.GetStoreSlice(template.CollectionStoreKey)...)
	}

	ids = append(ids, collectBlockLinks(s)...)
	ids = append(ids, sb.collectRelationLinks(s)...)

	return lo.Uniq(ids)
}

func collectBlockLinks(s *state.State) (ids []string) {
	err := s.Iterate(func(b simple.Block) (isContinue bool) {
		if f := b.Model().GetFile(); f != nil {
			if f.TargetObjectId != "" && f.Type != model.BlockContentFile_Image {
				ids = append(ids, f.TargetObjectId)
			}
			return true
		}
		// Include only link to target object
		if dv := b.Model().GetDataview(); dv != nil {
			if dv.TargetObjectId != "" {
				ids = append(ids, dv.TargetObjectId)
			}

			return true
		}

		if ls, ok := b.(linkSource); ok {
			ids = ls.FillSmartIds(ids)
		}
		return true
	})
	if err != nil {
		log.With("objectID", s.RootId()).Errorf("failed to iterate over simple blocks: %s", err)
	}
	return
}

func (sb *smartBlock) collectRelationLinks(s *state.State) (ids []string) {
	det := s.CombinedDetails()
	includeRelations := sb.includeRelationObjectsAsDependents

	for _, rel := range s.GetRelationLinks() {
		if includeRelations {
			relId, err := sb.space.GetRelationIdByKey(context.TODO(), domain.RelationKey(rel.Key))
			if err != nil {
				log.With("objectID", s.RootId()).Errorf("failed to derive object id for relation: %s", err)
				continue
			}
			ids = append(ids, relId)
		}

		// handle corner cases: first for specific formats and system relations
		if rel.Format != model.RelationFormat_object || bundle.IsSystemRelation(domain.RelationKey(rel.Key)) {
			continue
		}

		// Do not include hidden relations. Only bundled relations can be hidden, so we don't need
		// to request relations from object store.
		if r, err := bundle.GetRelation(domain.RelationKey(rel.Key)); err == nil && r.Hidden {
			continue
		}

		// Add all object relation values as dependents
		for _, targetID := range det.GetStringListOrDefault(domain.RelationKey(rel.Key), nil) {
			if targetID != "" {
				ids = append(ids, targetID)
			}
		}
	}
	return
}

func isBacklinksChanged(msgs []simple.EventMessage) bool {
	for _, msg := range msgs {
		if amend, ok := msg.Msg.Value.(*pb.EventMessageValueOfObjectDetailsAmend); ok {
			for _, detail := range amend.ObjectDetailsAmend.Details {
				if detail.Key == bundle.RelationKeyBacklinks.String() {
					return true
				}
			}
		}
	}
	return false
}
