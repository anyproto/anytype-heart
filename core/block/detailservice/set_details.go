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

func (s *service) SetIsArchived(sctx session.Context, ctx context.Context, objectId string, isArchived bool, skipCascade bool) error {
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
	return s.setIsArchivedForObjects(sctx, ctx, spaceID, []string{objectId}, isArchived, skipCascade)
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

func (s *service) SetListIsArchived(sctx session.Context, ctx context.Context, objectIds []string, isArchived bool, skipCascade bool) error {
	objectIdsPerSpace, err := s.partitionObjectIdsBySpaceId(objectIds)
	if err != nil {
		return fmt.Errorf("partition object ids by spaces: %w", err)
	}

	var (
		resultErr  error
		anySucceed bool
	)
	for spaceId, objectIdsOfThisSpace := range objectIdsPerSpace {
		err = s.setIsArchivedForObjects(sctx, ctx, spaceId, objectIdsOfThisSpace, isArchived, skipCascade)
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

func (s *service) setIsArchivedForObjects(sctx session.Context, ctx context.Context, spaceId string, objectIds []string, isArchived bool, skipCascade bool) error {
	if skipCascade {
		// pure archive — no orphan cascade (no file auto-archive, no events); reuse the NoGC path.
		return s.setIsArchivedForObjectsNoGC(ctx, spaceId, objectIds, isArchived)
	}

	spc, err := s.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}

	gcFiles, candidatesByContext := s.triggerGCOnArchive(spaceId, objectIds, isArchived)
	s.appendGCEvents(sctx, gcFiles, candidatesByContext, objectIds, isArchived)

	// Merge explicit IDs and GC-collected files so they are all archived in one operation.
	allIds := append(objectIds, gcFiles...)

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
		// Success is judged over the EXPLICIT ids only: the caller asked
		// about the objects, not their orphans, and a cascaded file
		// archiving while every requested object was refused must not turn
		// into a success return (an API DELETE would answer 200 with the
		// object still there). The cascade stays best-effort — its failures
		// are logged, never returned.
		explicit := make(map[string]struct{}, len(objectIds))
		for _, id := range objectIds {
			explicit[id] = struct{}{}
		}
		var explicitIds, cascadeIds []string
		for _, id := range ids {
			if _, ok := explicit[id]; ok {
				explicitIds = append(explicitIds, id)
			} else {
				cascadeIds = append(cascadeIds, id)
			}
		}
		anySucceed, err := s.modifyArchiveLinks(ctx, archive, isArchived, explicitIds...)
		if err != nil {
			log.Warn("failed to archive", zap.Error(err))
		}
		if _, cascadeErr := s.modifyArchiveLinks(ctx, archive, isArchived, cascadeIds...); cascadeErr != nil {
			log.Warn("failed to archive cascaded files", zap.Error(cascadeErr))
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
// triggerGCOnArchive runs GC for each objectId and returns the aggregated level-1 files to
// archive plus, keyed by originating object, the orphan candidates to surface to the user.
func (s *service) triggerGCOnArchive(spaceId string, objectIds []string, isArchived bool) (files []string, candidatesByContext map[string][]string) {
	if len(objectIds) == 0 {
		return nil, nil
	}
	candidatesByContext = make(map[string][]string)
	for _, objId := range objectIds {
		res, err := s.objectGC.CheckObjectsOnObjectArchived(spaceId, objId, isArchived)
		if err != nil {
			log.Error("GC failed for archived object", zap.String("objectId", objId), zap.Error(err))
			continue
		}
		files = append(files, res.Files...)
		if len(res.Candidates) > 0 {
			candidatesByContext[objId] = res.Candidates
		}
	}
	return files, candidatesByContext
}

// appendGCEvents emits the file auto-archive/restore event plus one CleanupSuggestion event per
// originating context, then strips explicitIds so user-requested objects are not double-reported.
func (s *service) appendGCEvents(sctx session.Context, gcFiles []string, candidatesByContext map[string][]string, explicitIds []string, isArchived bool) {
	if sctx == nil {
		return
	}
	msgs := sctx.GetMessages()
	changed := false
	if len(gcFiles) > 0 {
		if isArchived {
			msgs = append(msgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfObjectAutoArchive{
					ObjectAutoArchive: &pb.EventObjectAutoArchive{ObjectIds: gcFiles},
				},
			})
		} else {
			msgs = append(msgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfObjectAutoRestore{
					ObjectAutoRestore: &pb.EventObjectAutoRestore{ObjectIds: gcFiles},
				},
			})
		}
		changed = true
	}
	// CleanupSuggestion is only emitted on the archive direction.
	if isArchived {
		for contextId, candidates := range candidatesByContext {
			if len(candidates) == 0 {
				continue
			}
			msgs = append(msgs, &pb.EventMessage{
				Value: &pb.EventMessageValueOfObjectCleanupSuggestion{
					ObjectCleanupSuggestion: &pb.EventObjectCleanupSuggestion{
						ObjectIds: candidates,
						ContextId: contextId,
						Trigger:   pb.EventObjectCleanupSuggestion_archive,
					},
				},
			})
			changed = true
		}
	}
	if changed {
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

// SetCreatedInContextIgnored excludes objects from cleanup suggestions and from automatic
// context-driven archival, by ignoring their createdInContext link. The detail is written directly on
// the state with a non-user change type, which skips the lastModifiedDate bump (SetLastModified is
// only called for domain.ChangeTypeUserChange) while still producing a real, syncing CRDT change.
func (s *service) SetCreatedInContextIgnored(ctx context.Context, objectIds []string, ignored bool) error {
	var (
		resultErr  error
		anySucceed bool
	)
	for _, objectId := range objectIds {
		err := cache.Do(s.objectGetter, objectId, func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			st.SetDetail(bundle.RelationKeyCreatedInContextIgnored, domain.Bool(ignored))
			st.SetChangeType(domain.ChangeTypeCreatedInContext)
			return sb.Apply(st)
		})
		if err != nil {
			log.Error("failed to set createdInContextIgnored", zap.String("objectId", objectId), zap.Error(err))
			resultErr = errors.Join(resultErr, fmt.Errorf("set createdInContextIgnored on %s: %w", objectId, err))
			continue
		}
		anySucceed = true
	}
	if anySucceed {
		return nil
	}
	return resultErr
}
