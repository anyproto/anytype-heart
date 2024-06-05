package block

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *Service) DeleteObjectByFullID(id domain.FullID) (err error) {
	var sbType coresb.SmartBlockType
	spc, err := s.spaceService.Get(context.Background(), id.SpaceID)
	if err != nil {
		return
	}
	err = spc.Do(id.ObjectID, func(b smartblock.SmartBlock) error {
		if err = b.Restrictions().Object.Check(model.Restrictions_Delete); err != nil {
			return err
		}
		sbType = b.Type()
		return nil
	})
	if err != nil {
		return
	}

	switch sbType {
	case coresb.SmartBlockTypeObjectType,
		coresb.SmartBlockTypeRelation,
		coresb.SmartBlockTypeRelationOption,
		coresb.SmartBlockTypeTemplate:
		return s.deleteDerivedObject(id, spc)
	case coresb.SmartBlockTypeSubObject:
		return fmt.Errorf("subobjects deprecated")
	case coresb.SmartBlockTypeFileObject:
		err = s.fileObjectService.DeleteFileData(id.ObjectID)
		if err != nil {
			return fmt.Errorf("delete file data: %w", err)
		}
		// this will call DeleteTree asynchronously in the end
		return spc.DeleteTree(context.Background(), id.ObjectID)
	default:
		// this will call DeleteTree asynchronously in the end
		return spc.DeleteTree(context.Background(), id.ObjectID)
	}
}

func (s *Service) deleteDerivedObject(id domain.FullID, spc clientspace.Space) (err error) {
	var (
		relationKey string
		sbType      coresb.SmartBlockType
	)
	err = spc.Do(id.ObjectID, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyIsUninstalled, pbtypes.Bool(true))
		if sbType == coresb.SmartBlockTypeRelation {
			relationKey = pbtypes.GetString(st.Details(), bundle.RelationKeyRelationKey.String())
		}
		return b.Apply(st)
	})
	if err != nil {
		return fmt.Errorf("set isUninstalled flag: %w", err)
	}
	err = s.OnDelete(id, nil)
	if err != nil {
		return fmt.Errorf("on delete: %w", err)
	}
	if sbType == coresb.SmartBlockTypeRelation {
		err := s.deleteRelationOptions(relationKey)
		if err != nil {
			return fmt.Errorf("failed to delete relation options of deleted relation: %w", err)
		}
	}
	sendOnRemoveEvent(s.eventSender, id.ObjectID)
	return spc.Remove(context.Background(), id.ObjectID)
}

func (s *Service) deleteRelationOptions(relationKey string) error {
	relationOptions, _, err := s.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
			},
			{
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(relationKey),
			},
		},
	})
	if err != nil {
		return err
	}
	for _, id := range relationOptions {
		err := s.DeleteObject(id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DeleteObject(objectId string) (err error) {
	spaceId, err := s.resolver.ResolveSpaceID(objectId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	return s.DeleteObjectByFullID(domain.FullID{SpaceID: spaceId, ObjectID: objectId})
}

func (s *Service) OnDelete(id domain.FullID, workspaceRemove func() error) error {
	var (
		isFavorite bool
	)

	err := s.DoFullId(id, func(b smartblock.SmartBlock) error {
		b.ObjectCloseAllSessions()
		st := b.NewState()
		isFavorite = pbtypes.GetBool(st.LocalDetails(), bundle.RelationKeyIsFavorite.String())
		if isFavorite {
			_ = s.SetPageIsFavorite(pb.RpcObjectSetIsFavoriteRequest{IsFavorite: false, ContextId: id.ObjectID})
		}
		b.SetIsDeleted()
		if workspaceRemove != nil {
			return workspaceRemove()
		}
		return nil
	})
	if err != nil {
		log.Error("failed to perform delete operation on object", zap.Error(err))
	}
	if err := s.objectStore.DeleteObject(id); err != nil {
		return fmt.Errorf("delete object from local store: %w", err)
	}

	return nil
}

func sendOnRemoveEvent(eventSender event.Sender, ids ...string) {
	eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfObjectRemove{
					ObjectRemove: &pb.EventObjectRemove{
						Ids: ids,
					},
				},
			},
		},
	})
}

func sendOnCloseEvent(eventSender event.Sender, id string, closer pb.EventObjectCloseCloser) {
	eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfObjectClose{
					ObjectClose: &pb.EventObjectClose{
						Id:     id,
						Closer: closer,
					},
				},
			},
		},
	})
}
