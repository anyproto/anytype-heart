package block

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *Service) DeleteObjectByFullID(id domain.FullID) error {
	var sbType coresb.SmartBlockType
	spc, err := s.spaceService.Get(context.Background(), id.SpaceID)
	if err != nil {
		return err
	}
	err = spc.Do(id.ObjectID, func(b smartblock.SmartBlock) error {
		if err = b.Restrictions().Object.Check(model.Restrictions_Delete); err != nil {
			return err
		}
		sbType = b.Type()
		return nil
	})
	if err != nil {
		return err
	}
	switch sbType {
	case coresb.SmartBlockTypeObjectType,
		coresb.SmartBlockTypeRelation,
		coresb.SmartBlockTypeRelationOption,
		coresb.SmartBlockTypeTemplate:
		err = s.deleteDerivedObject(id, spc)
	case coresb.SmartBlockTypeSubObject:
		return fmt.Errorf("subobjects deprecated")
	case coresb.SmartBlockTypeFileObject:
		err = s.fileObjectService.DeleteFileData(id.SpaceID, id.ObjectID)
		if err != nil && !errors.Is(err, filemodels.ErrEmptyFileId) {
			return fmt.Errorf("delete file data: %w", err)
		}
		err = spc.DeleteTree(context.Background(), id.ObjectID)
	default:
		err = spc.DeleteTree(context.Background(), id.ObjectID)
	}
	if err != nil {
		return err
	}
	sendOnRemoveEvent(s.eventSender, id.ObjectID)
	// Remove from cache
	err = spc.Remove(context.Background(), id.ObjectID)
	if err != nil {
		return err
	}
	return nil
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
		err := s.deleteRelationOptions(id.SpaceID, relationKey)
		if err != nil {
			return fmt.Errorf("failed to delete relation options of deleted relation: %w", err)
		}
	}
	return nil
}

func (s *Service) deleteRelationOptions(spaceId string, relationKey string) error {
	relationOptions, _, err := s.objectStore.SpaceStore(spaceId).QueryObjectIDs(database.Query{
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
	err := s.DoFullId(id, func(b smartblock.SmartBlock) error {
		b.ObjectCloseAllSessions()
		st := b.NewState()
		isFavorite := pbtypes.GetBool(st.LocalDetails(), bundle.RelationKeyIsFavorite.String())
		if err := s.detailsService.SetIsFavorite(id.ObjectID, isFavorite); err != nil {
			log.With("objectId", id).Errorf("failed to favorite object: %v", err)
		}
		b.SetIsDeleted()
		if workspaceRemove != nil {
			return workspaceRemove()
		}
		return nil
	})
	if err != nil {
		log.With("error", err, "objectId", id.ObjectID).Error("failed to perform delete operation on object")
	}
	if err := s.objectStore.SpaceStore(id.SpaceID).DeleteObject(id.ObjectID); err != nil {
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
