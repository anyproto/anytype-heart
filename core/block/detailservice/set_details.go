package detailservice

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/blockcollection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

var ErrUnexpectedBlockType = errors.New("unexpected block type")

func (s *service) SetSpaceInfo(spaceId string, details *domain.Details) error {
	ctx := context.TODO()
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	workspaceId := spc.DerivedIDs().Workspace

	setDetails := make([]domain.Detail, 0, details.Len())
	for k, v := range details.Iterate() {
		setDetails = append(setDetails, domain.Detail{
			Key:   k,
			Value: v,
		})
	}
	return s.SetDetails(nil, workspaceId, setDetails)
}

func (s *service) SetWorkspaceDashboardId(ctx session.Context, workspaceId string, id string) (setId string, err error) {
	err = cache.Do(s.objectGetter, workspaceId, func(ws *editor.Workspaces) error {
		if ws.Type() != coresb.SmartBlockTypeWorkspace {
			return ErrUnexpectedBlockType
		}
		if err = ws.SetDetails(ctx, []domain.Detail{
			{
				Key:   bundle.RelationKeySpaceDashboardId,
				Value: domain.StringList([]string{id}),
			},
		}, false); err != nil {
			return err
		}
		return nil
	})
	return id, err
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

func (s *service) SetIsArchived(objectId string, isArchived bool) error {
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
	if err := s.checkArchivedRestriction(isArchived, objectId); err != nil {
		return err
	}
	return s.objectLinksCollectionModify(spc.DerivedIDs().Archive, objectId, isArchived)
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

func (s *service) SetListIsArchived(objectIds []string, isArchived bool) error {
	objectIdsPerSpace, err := s.partitionObjectIdsBySpaceId(objectIds)
	if err != nil {
		return fmt.Errorf("partition object ids by spaces: %w", err)
	}

	var (
		resultErr  error
		anySucceed bool
	)
	for spaceId, objectIdsOfThisSpace := range objectIdsPerSpace {
		err = s.setIsArchivedForObjects(spaceId, objectIdsOfThisSpace, isArchived)
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

func (s *service) checkArchivedRestriction(isArchived bool, objectId string) error {
	if !isArchived {
		return nil
	}
	return cache.Do(s.objectGetter, objectId, func(sb smartblock.SmartBlock) error {
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

func (s *service) setIsArchivedForObjects(spaceId string, objectIds []string, isArchived bool) error {
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
					// avoid archive system objects including archive itself
					return false
				}
			}
			return true
		})
		anySucceed, err := s.modifyArchiveLinks(archive, isArchived, ids...)

		if err != nil {
			log.Warn("failed to archive", zap.Error(err))
		}
		if anySucceed {
			return nil
		}
		return err
	})
}

func (s *service) modifyArchiveLinks(
	coll blockcollection.Collection, value bool, ids ...string,
) (anySucceed bool, resultErr error) {
	for _, id := range ids {
		err := s.checkArchivedRestriction(value, id)
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
