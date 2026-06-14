package detailservice

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/blockcollection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/objectgc"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/util/slice"
)

var ErrHomepageChangeRestricted = errors.New("homepage change is restricted for 1-on-1 spaces")

func (s *service) SetSpaceInfo(spaceId string, details *domain.Details) error {
	spc, err := s.spaceService.Get(s.componentCtx, spaceId)
	if err != nil {
		return err
	}
	workspaceId := spc.DerivedIDs().Workspace

	setDetails := make([]domain.Detail, 0, details.Len())
	for k, v := range details.Iterate() {
		if k == bundle.RelationKeyHomepage {
			if err = s.validateHomepage(spc, v); err != nil {
				return fmt.Errorf("validate homepage: %w", err)
			}
		}
		setDetails = append(setDetails, domain.Detail{
			Key:   k,
			Value: v,
		})
	}
	return s.SetDetails(nil, workspaceId, setDetails)
}

func (s *service) SetIsFavorite(objectId string, isFavorite bool) error {
	spaceID, err := s.resolver.ResolveSpaceID(objectId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	spc, err := s.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	if err = s.objectLinksCollectionModify(spc.DerivedIDs().Home, objectId, isFavorite); err != nil {
		return err
	}
	return nil
}

func (s *service) SetIsArchived(sctx session.Context, ctx context.Context, objectId string, isArchived bool) error {
	spaceID, err := s.resolver.ResolveSpaceID(objectId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	spc, err := s.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	if objectId == spc.DerivedIDs().Archive {
		return fmt.Errorf("can't archive archive itself")
	}
	return s.setIsArchivedForObjects(sctx, ctx, spaceID, []string{objectId}, isArchived)
}

func (s *service) SetListIsFavorite(objectIds []string, isFavorite bool) error {
	objectIdsPerSpace, err := s.partitionObjectIdsBySpaceId(objectIds)
	if err != nil {
		return fmt.Errorf("partition object ids by spaces: %w", err)
	}

	var (
		anySucceed  bool
		resultError error
	)
	for spaceId, objectIds := range objectIdsPerSpace {
		ids, err := s.store.SpaceIndex(spaceId).HasIds(objectIds)
		if err != nil {
			return err
		}

		for _, id := range ids {
			// TODO Set list of ids at once
			err := s.SetIsFavorite(id, isFavorite)
			if err != nil {
				log.Error("failed to favorite object", zap.String("objectId", id), zap.Error(err))
				resultError = errors.Join(resultError, err)
			} else {
				anySucceed = true
			}
		}

	}
	if resultError != nil {
		log.Warn("failed to set objects as favorite", zap.Error(resultError))
	}
	if anySucceed {
		return nil
	}
	return resultError
}

func (s *service) SetListIsArchived(sctx session.Context, ctx context.Context, objectIds []string, isArchived bool) error {
	objectIdsPerSpace, err := s.partitionObjectIdsBySpaceId(objectIds)
	if err != nil {
		return fmt.Errorf("partition object ids by spaces: %w", err)
	}

	var (
		resultErr  error
		anySucceed bool
	)
	for spaceId, objectIdsOfThisSpace := range objectIdsPerSpace {
		err = s.setIsArchivedForObjects(sctx, ctx, spaceId, objectIdsOfThisSpace, isArchived)
		if err != nil {
			log.Error("failed to set isArchived to objects", zap.String("spaceId", spaceId),
				zap.Strings("objectIds", objectIdsOfThisSpace), zap.Bool("isArchived", isArchived), zap.Error(err))
			resultErr = errors.Join(resultErr, err)
			continue
		}
		anySucceed = true
	}
	if anySucceed {
		return nil
	}
	return resultErr
}

// SetListIsArchivedNoGC archives/unarchives objects without triggering another GC pass.
// Used by the GC itself for its batch write to prevent recursive re-entry.
func (s *service) SetListIsArchivedNoGC(ctx context.Context, objectIds []string, isArchived bool) error {
	objectIdsPerSpace, err := s.partitionObjectIdsBySpaceId(objectIds)
	if err != nil {
		return fmt.Errorf("partition object ids by spaces: %w", err)
	}

	var (
		resultErr  error
		anySucceed bool
	)
	for spaceId, objectIdsOfThisSpace := range objectIdsPerSpace {
		err = s.setIsArchivedForObjectsNoGC(ctx, spaceId, objectIdsOfThisSpace, isArchived)
		if err != nil {
			log.Error("failed to set isArchived to objects (no-gc)", zap.String("spaceId", spaceId),
				zap.Strings("objectIds", objectIdsOfThisSpace), zap.Bool("isArchived", isArchived), zap.Error(err))
			resultErr = errors.Join(resultErr, err)
			continue
		}
		anySucceed = true
	}
	if anySucceed {
		return nil
	}
	return resultErr
}

func (s *service) checkArchivedRestriction(ctx context.Context, isArchived bool, objectId string) error {
	if !isArchived {
		return nil
	}
	return cache.Do(s.objectGetter, objectId, func(sb smartblock.SmartBlock) error {
		if sb.Type() == coresb.SmartBlockTypeFileObject {
			err := s.fileService.CanDeleteFile(ctx, objectId)
			if err != nil {
				return err
			}
		}

		return restriction.CheckRestrictions(sb, model.Restrictions_Delete)
	})
}

func (s *service) objectLinksCollectionModify(collectionId string, objectId string, value bool) error {
	if objectId == collectionId {
		return fmt.Errorf("can't add links collection to itself")
	}
	return cache.Do(s.objectGetter, collectionId, func(b smartblock.SmartBlock) error {
		coll, ok := b.(blockcollection.Collection)
		if !ok {
			return fmt.Errorf("unsupported sb block type: %T", b)
		}
		if value {
			return coll.AddObject(objectId)
		} else {
			return coll.RemoveObject(objectId)
		}
	})
}

func (s *service) partitionObjectIdsBySpaceId(objectIds []string) (map[string][]string, error) {
	res := make(map[string][]string, len(objectIds))
	for _, objectId := range objectIds {
		spaceId, err := s.resolver.ResolveSpaceID(objectId)
		if err != nil {
			return nil, fmt.Errorf("resolve spaceId: %w", err)
		}
		res[spaceId] = append(res[spaceId], objectId)
	}
	return res, nil
}

func (s *service) setIsArchivedForObjects(sctx session.Context, ctx context.Context, spaceId string, objectIds []string, isArchived bool) error {
	spc, err := s.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}

	gcIds := s.triggerGCOnArchive(spaceId, objectIds, isArchived)
	s.appendGCEvent(sctx, gcIds, objectIds, isArchived)

	// Merge explicit IDs and GC-collected children so they are all archived in one operation.
	allIds := append(objectIds, gcIds...)

	return cache.Do(s.objectGetter, spc.DerivedIDs().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(blockcollection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		ids, err := s.store.SpaceIndex(spaceId).HasIds(allIds)
		if err != nil {
			return err
		}

		ids = slice.Filter(ids, func(id string) bool {
			for _, objId := range spc.DerivedIDs().IDsWithSystemTypesAndRelations() {
				if id == objId {
					return false
				}
			}
			return true
		})
		anySucceed, err := s.modifyArchiveLinks(ctx, archive, isArchived, ids...)

		if err != nil {
			log.Warn("failed to archive", zap.Error(err))
		}
		if anySucceed {
			return nil
		}
		return err
	})
}

// setIsArchivedForObjectsNoGC is identical to setIsArchivedForObjects but does NOT call
// triggerGCOnArchive, preventing recursive GC re-entry when the GC itself archives objects.
func (s *service) setIsArchivedForObjectsNoGC(ctx context.Context, spaceId string, objectIds []string, isArchived bool) error {
	spc, err := s.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}

	return cache.Do(s.objectGetter, spc.DerivedIDs().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(blockcollection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		ids, err := s.store.SpaceIndex(spaceId).HasIds(objectIds)
		if err != nil {
			return err
		}

		ids = slice.Filter(ids, func(id string) bool {
			for _, objId := range spc.DerivedIDs().IDsWithSystemTypesAndRelations() {
				if id == objId {
					return false
				}
			}
			return true
		})
		anySucceed, err := s.modifyArchiveLinks(ctx, archive, isArchived, ids...)

		if err != nil {
			log.Warn("failed to archive (no-gc)", zap.Error(err))
		}
		if anySucceed {
			return nil
		}
		return err
	})
}

func (s *service) modifyArchiveLinks(ctx context.Context, coll blockcollection.Collection, value bool, ids ...string) (anySucceed bool, resultErr error) {
	for _, id := range ids {
		err := s.checkArchivedRestriction(ctx, value, id)
		if err == nil {
			if value {
				err = coll.AddObject(id)
			} else {
				err = coll.RemoveObject(id)
			}
		}
		if err != nil {
			resultErr = errors.Join(resultErr, err)
			continue
		}
		anySucceed = true
	}
	return
}

// triggerGCOnArchive runs GC for all children of objectIds that have no other backlinks.
// It returns the IDs of objects that were archived or restored; the caller is responsible
// for emitting events and filtering explicit IDs from the session context.
func (s *service) triggerGCOnArchive(spaceId string, objectIds []string, isArchived bool) []string {
	if len(objectIds) == 0 {
		return nil
	}
	var allFiles []string
	for _, objId := range objectIds {
		res, err := s.objectGC.CheckObjectsOnObjectArchived(spaceId, objId, isArchived)
		if err != nil {
			log.Error("GC failed for archived object", zap.String("objectId", objId), zap.Error(err))
			continue
		}
		allFiles = append(allFiles, res.Files...)
	}
	return allFiles
}

// appendGCEvent emits a single auto-archive or auto-restore event for gcIds into sctx,
// then strips explicitIds from all GC events so user-requested objects are not double-reported.
func (s *service) appendGCEvent(sctx session.Context, gcIds []string, explicitIds []string, isArchived bool) {
	if sctx != nil && len(gcIds) > 0 {
		msgs := sctx.GetMessages()
		if isArchived {
			msgs = append(msgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfObjectAutoArchive{
					ObjectAutoArchive: &pb.EventObjectAutoArchive{ObjectIds: gcIds},
				},
			})
		} else {
			msgs = append(msgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfObjectAutoRestore{
					ObjectAutoRestore: &pb.EventObjectAutoRestore{ObjectIds: gcIds},
				},
			})
		}
		sctx.SetMessages(sctx.ObjectID(), msgs)
	}
	objectgc.FilterExplicitIds(sctx, explicitIds)
}

func (s *service) validateHomepage(spc clientspace.Space, homepageValue domain.Value) error {
	if !homepageValue.IsString() {
		return fmt.Errorf("invalid homepage value type: %s", homepageValue.Type().String())
	}
	homepage := homepageValue.String()

	if spc.SpaceType() == spacedomain.SpaceTypeOneToOne {
		return ErrHomepageChangeRestricted
	}

	if domain.IsHomepageConstant(homepage) {
		return nil
	}
	exists, err := s.store.SpaceIndex(spc.Id()).HasIds([]string{homepage})
	if err != nil {
		return fmt.Errorf("check homepage object existence: %w", err)
	}
	if len(exists) == 0 {
		return fmt.Errorf("homepage object %s not found in space %s", homepage, spc.Id())
	}
	return nil
}
