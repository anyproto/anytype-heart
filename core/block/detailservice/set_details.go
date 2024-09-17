package detailservice

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var ErrUnexpectedBlockType = errors.New("unexpected block type")

func (s *service) SetSpaceInfo(spaceId string, details *types.Struct) error {
	ctx := context.TODO()
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	workspaceId := spc.DerivedIDs().Workspace

	setDetails := make([]*model.Detail, 0, len(details.GetFields()))
	for k, v := range details.GetFields() {
		setDetails = append(setDetails, &model.Detail{
			Key:   k,
			Value: v,
		})
	}
	return s.SetDetails(nil, workspaceId, setDetails)
}

func (s *service) SetSource(ctx session.Context, objectId string, source []string) error {
	return cache.Do(s, objectId, func(sb smartblock.SmartBlock) error {
		st := sb.NewStateCtx(ctx)
		// nolint:errcheck
		_ = st.Iterate(func(b simple.Block) (isContinue bool) {
			if dv := b.Model().GetDataview(); dv != nil {
				for _, view := range dv.Views {
					view.DefaultTemplateId = ""
					view.DefaultObjectTypeId = ""
				}
				st.Set(b)
				return false
			}
			return true
		})
		st.SetDetailAndBundledRelation(bundle.RelationKeySetOf, pbtypes.StringList(source))

		flags := internalflag.NewFromState(st)
		// set with source is no longer empty
		flags.Remove(model.InternalFlag_editorDeleteEmpty)
		flags.AddToState(st)

		return sb.Apply(st, smartblock.NoRestrictions, smartblock.KeepInternalFlags)
	})
}

func (s *service) SetWorkspaceDashboardId(ctx session.Context, workspaceId string, id string) (setId string, err error) {
	err = cache.Do(s, workspaceId, func(ws *editor.Workspaces) error {
		if ws.Type() != coresb.SmartBlockTypeWorkspace {
			return ErrUnexpectedBlockType
		}
		if err = ws.SetDetails(ctx, []*model.Detail{
			{
				Key:   bundle.RelationKeySpaceDashboardId.String(),
				Value: pbtypes.String(id),
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
	return s.objectLinksCollectionModify(spc.DerivedIDs().Home, objectId, isFavorite)
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
	if err := s.checkArchivedRestriction(isArchived, objectId); err != nil {
		return err
	}
	return s.objectLinksCollectionModify(spc.DerivedIDs().Archive, objectId, isArchived)
}

func (s *service) SetListIsFavorite(objectIds []string, isFavorite bool) error {
	ids, err := s.store.HasIDs(objectIds...)
	if err != nil {
		return err
	}
	var (
		anySucceed  bool
		resultError error
	)
	for _, id := range ids {
		err := s.SetIsFavorite(id, isFavorite)
		if err != nil {
			log.Error("failed to favorite object", zap.String("objectId", id), zap.Error(err))
			resultError = errors.Join(resultError, err)
		} else {
			anySucceed = true
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
	objectIDsPerSpace, err := s.partitionObjectIDsBySpaceID(objectIds)
	if err != nil {
		return fmt.Errorf("partition object ids by spaces: %w", err)
	}

	var (
		multiErr   multierror.Error
		anySucceed bool
	)
	for spaceID, objectIDs := range objectIDsPerSpace {
		err = s.setIsArchivedForObjects(spaceID, objectIDs, isArchived)
		if err != nil {
			log.Error("failed to set isArchived to objects", zap.String("spaceId", spaceID),
				zap.Strings("objectIDs", objectIDs), zap.Bool("isArchived", isArchived), zap.Error(err))
			multiErr.Errors = append(multiErr.Errors, err)
		} else {
			anySucceed = true
		}
	}
	if anySucceed {
		return nil
	}
	return multiErr.ErrorOrNil()
}

func (s *service) checkArchivedRestriction(isArchived bool, objectId string) error {
	if !isArchived {
		return nil
	}
	return cache.Do(s, objectId, func(sb smartblock.SmartBlock) error {
		return s.restriction.CheckRestrictions(sb, model.Restrictions_Delete)
	})
}

func (s *service) objectLinksCollectionModify(collectionId string, objectId string, value bool) error {
	return cache.Do(s, collectionId, func(b smartblock.SmartBlock) error {
		coll, ok := b.(collection.Collection)
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

func (s *service) partitionObjectIDsBySpaceID(objectIDs []string) (map[string][]string, error) {
	res := map[string][]string{}
	for _, objectID := range objectIDs {
		spaceID, err := s.resolver.ResolveSpaceID(objectID)
		if err != nil {
			return nil, fmt.Errorf("resolve spaceID: %w", err)
		}
		res[spaceID] = append(res[spaceID], objectID)
	}
	return res, nil
}

func (s *service) setIsArchivedForObjects(spaceID string, objectIDs []string, isArchived bool) error {
	spc, err := s.spaceService.Get(context.Background(), spaceID)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	return cache.Do(s, spc.DerivedIDs().Archive, func(b smartblock.SmartBlock) error {
		archive, ok := b.(collection.Collection)
		if !ok {
			return fmt.Errorf("unexpected archive block type: %T", b)
		}

		var multiErr multierror.Error
		var anySucceed bool
		ids, err := s.store.HasIDs(objectIDs...)
		if err != nil {
			return err
		}
		for _, id := range ids {
			var err error
			if restrErr := s.checkArchivedRestriction(isArchived, id); restrErr != nil {
				err = restrErr
			} else {
				if isArchived {
					err = archive.AddObject(id)
				} else {
					err = archive.RemoveObject(id)
				}
			}
			if err != nil {
				log.Error("failed to set isArchived", zap.String("objectId", id), zap.Bool("isArchived", isArchived), zap.Error(err))
				multiErr.Errors = append(multiErr.Errors, err)
				continue
			}
			anySucceed = true
		}

		if err := multiErr.ErrorOrNil(); err != nil {
			log.Warn("failed to archive", zap.Error(err))
		}
		if anySucceed {
			return nil
		}
		return multiErr.ErrorOrNil()
	})
}
