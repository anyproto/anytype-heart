package block

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/spacestorage"

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

	entry, err := spc.Storage().HeadStorage().GetEntry(s.componentCtx, id.ObjectID)
	if err != nil {
		return fmt.Errorf("get head entry: %w", err)
	}

	switch sbType {
	case coresb.SmartBlockTypeObjectType,
		coresb.SmartBlockTypeRelation,
		coresb.SmartBlockTypeRelationOption,
		coresb.SmartBlockTypeTemplate:
		err = s.deleteDerivedObject(id, sbType, spc)
	case coresb.SmartBlockTypeSubObject:
		return fmt.Errorf("subobjects deprecated")
	case coresb.SmartBlockTypeFileObject:
		err = s.fileObjectService.DeleteFileData(id.SpaceID, id.ObjectID)
		if err != nil && !errors.Is(err, filemodels.ErrEmptyFileId) {
			return fmt.Errorf("delete file data: %w", err)
		}
		err = spc.DeleteTree(context.Background(), id.ObjectID)
	default:
		if entry.IsDerived {
			err = s.deleteDerivedObject(id, sbType, spc)
		} else {
			err = spc.DeleteTree(context.Background(), id.ObjectID)
		}
	}
	if err != nil {
		return err
	}
	s.sendOnRemoveEvent(id.SpaceID, id.ObjectID)
	// Remove from cache
	err = spc.Remove(context.Background(), id.ObjectID)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) deleteDerivedObject(id domain.FullID, sbType coresb.SmartBlockType, spc clientspace.Space) (err error) {
	var relationKey, targetTypeId string
	err = spc.Do(id.ObjectID, func(b smartblock.SmartBlock) error {
		st := b.NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyIsUninstalled, domain.Bool(true))
		switch sbType {
		case coresb.SmartBlockTypeRelation:
			relationKey = st.Details().GetString(bundle.RelationKeyRelationKey)
		case coresb.SmartBlockTypeTemplate:
			targetTypeId = st.Details().GetString(bundle.RelationKeyTargetObjectType)
		}
		return b.Apply(st)
	})
	if err != nil {
		return fmt.Errorf("set isUninstalled flag: %w", err)
	}
	err = s.BeforeDelete(id, nil)
	if err != nil {
		return fmt.Errorf("on delete: %w", err)
	}
	switch sbType {
	case coresb.SmartBlockTypeRelation:
		if err = s.deleteRelationOptions(id.SpaceID, relationKey); err != nil {
			return fmt.Errorf("failed to delete relation options of deleted relation: %w", err)
		}
	case coresb.SmartBlockTypeTemplate:
		if err = s.unsetDefaultTemplateId(id.ObjectID, targetTypeId, spc); err != nil {
			return fmt.Errorf("failed to reset default template id: %w", err)
		}
	}
	return nil
}

func (s *Service) deleteRelationOptions(spaceId string, relationKey string) error {
	relationOptions, _, err := s.objectStore.SpaceIndex(spaceId).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_relationOption),
			},
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(relationKey),
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

func (s *Service) unsetDefaultTemplateId(templateId, typeId string, spc clientspace.Space) error {
	records, err := s.objectStore.SpaceIndex(spc.Id()).QueryByIds([]string{typeId})
	if err != nil {
		return fmt.Errorf("failed to query type object: %w", err)
	}
	if len(records) != 1 {
		return fmt.Errorf("failed to query type object: 1 record expected")
	}
	if records[0].Details.GetString(bundle.RelationKeyDefaultTemplateId) != templateId {
		// template that is about to be deleted was not set as default for type, so we do nothing
		return nil
	}

	return spc.Do(typeId, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.SetDetail(bundle.RelationKeyDefaultTemplateId, domain.String(""))
		st.SetChangeType(domain.ChangeTypeLayoutSync)
		return sb.Apply(st)
	})
}

func (s *Service) DeleteObject(objectId string) (err error) {
	spaceId, err := s.resolver.ResolveSpaceID(objectId)
	if err != nil {
		return fmt.Errorf("resolve spaceID: %w", err)
	}
	return s.DeleteObjectByFullID(domain.FullID{SpaceID: spaceId, ObjectID: objectId})
}

func (s *Service) BeforeDelete(id domain.FullID, workspaceRemove func() error) error {
	err := s.DoFullId(id, func(b smartblock.SmartBlock) error {
		b.ObjectCloseAllSessions()
		st := b.NewState()
		isFavorite := st.LocalDetails().GetBool(bundle.RelationKeyIsFavorite)
		if err := s.detailsService.SetIsFavorite(id.ObjectID, isFavorite); err != nil {
			log.With("objectId", id).Errorf("failed to favorite object: %v", err)
		}
		b.SetIsDeleted()
		if workspaceRemove != nil {
			return workspaceRemove()
		}
		return nil
	})
	if err != nil && !errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) {
		log.With("error", err, "objectId", id.ObjectID).Error("failed to perform delete operation on object")
	}
	if err := s.objectStore.SpaceIndex(id.SpaceID).DeleteObject(id.ObjectID); err != nil {
		return fmt.Errorf("delete object from local store: %w", err)
	}

	return nil
}

func (s *Service) sendOnRemoveEvent(spaceId string, id string) {
	s.eventSender.Broadcast(event.NewEventSingleMessage(spaceId, &pb.EventMessageValueOfObjectRemove{
		ObjectRemove: &pb.EventObjectRemove{
			Ids: []string{id},
		},
	}))
}
