package smartblock

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (sb *smartBlock) updateBackLinks(details *types.Struct) {
	backLinks, err := sb.objectStore.GetInboundLinksByID(sb.Id())
	if err != nil {
		log.With("objectID", sb.Id()).Errorf("failed to get inbound links from object store: %s", err)
		return
	}
	details.Fields[bundle.RelationKeyBacklinks.String()] = pbtypes.StringList(backLinks)
}

func (sb *smartBlock) injectLinksDetails(s *state.State) {
	links := sb.navigationalLinks(s)
	links = slice.RemoveMut(links, sb.Id())

	currentLinks := pbtypes.GetStringList(s.LocalDetails(), bundle.RelationKeyLinks.String())

	addedLinks, deletedLinks := lo.Difference(links, currentLinks)
	if len(addedLinks)+len(deletedLinks) != 0 {
		go sb.addBacklinkToObjects(addedLinks, deletedLinks)
	}

	// todo: we need to move it to the injectDerivedDetails, but we don't call it now on apply
	s.SetLocalDetail(bundle.RelationKeyLinks.String(), pbtypes.StringList(links))
}

func (sb *smartBlock) addBacklinkToObjects(added, removed []string) {
	addBacklink := func(current *types.Struct) (*types.Struct, error) {
		if current == nil || current.Fields == nil {
			current = &types.Struct{
				Fields: map[string]*types.Value{},
			}
		}
		backlinks := pbtypes.GetStringList(current, bundle.RelationKeyBacklinks.String())
		backlinks = append(backlinks, sb.Id())
		current.Fields[bundle.RelationKeyBacklinks.String()] = pbtypes.StringList(backlinks)
		return current, nil
	}

	removeBacklink := func(current *types.Struct) (*types.Struct, error) {
		if current == nil || current.Fields == nil {
			return current, nil
		}
		backlinks := pbtypes.GetStringList(current, bundle.RelationKeyBacklinks.String())
		backlinks = slice.RemoveMut(backlinks, sb.Id())
		current.Fields[bundle.RelationKeyBacklinks.String()] = pbtypes.StringList(backlinks)
		return current, nil
	}

	for _, modification := range []struct {
		ids      []string
		modifier func(details *types.Struct) (*types.Struct, error)
	}{
		{added, addBacklink},
		{removed, removeBacklink},
	} {
		for _, id := range modification.ids {
			err := sb.Space().DoLockedIfNotExists(id, func() error {
				return sb.objectStore.ModifyObjectDetails(id, modification.modifier)
			})
			if err != nil && !errors.Is(err, ocache.ErrExists) {
				log.With("objectID", sb.Id()).Errorf("failed to update backlinks for object %s: %v", id, err)
			}
			if err = sb.Space().Do(id, func(b SmartBlock) error {
				details, err := modification.modifier(b.CombinedDetails())
				if err != nil {
					return err
				}

				return b.Apply(b.NewState().SetDetails(details), KeepInternalFlags)
			}); err != nil {
				log.With("objectID", sb.Id()).Errorf("failed to update backlinks for object %s: %v", id, err)
			}
		}
	}
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
			if f.Hash != "" && f.Type != model.BlockContentFile_Image {
				ids = append(ids, f.Hash)
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
		for _, targetID := range pbtypes.GetStringList(det, rel.Key) {
			if targetID != "" {
				ids = append(ids, targetID)
			}
		}
	}
	return
}
