package smartblock

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/app/ocache"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (sb *smartBlock) injectLocalDetails(s *state.State) error {
	details, err := sb.getDetailsFromStore()
	if err != nil {
		return err
	}

	details, hasPendingLocalDetails := sb.appendPendingDetails(details)

	// inject also derived keys, because it may be a good idea to have created date and creator cached,
	// so we don't need to traverse changes every time
	keys := bundle.LocalAndDerivedRelationKeys

	localDetailsFromStore := details.CopyOnlyKeys(keys...)

	s.AddLocalDetails(localDetailsFromStore)
	if p := s.ParentState(); p != nil && !hasPendingLocalDetails {
		// inject for both current and parent state
		p.AddLocalDetails(localDetailsFromStore)
	}

	err = sb.injectCreationInfo(s)
	if err != nil {
		log.With("objectID", sb.Id()).With("sbtype", sb.Type().String()).Errorf("failed to inject creation info: %s", err.Error())
	}
	return nil
}

func (sb *smartBlock) getDetailsFromStore() (*domain.Details, error) {
	storedDetails, err := sb.spaceIndex.GetDetails(sb.Id())
	if err != nil || storedDetails == nil {
		return nil, err
	}
	return storedDetails.Copy(), nil
}

func (sb *smartBlock) appendPendingDetails(details *domain.Details) (resultDetails *domain.Details, hasPendingLocalDetails bool) {
	// Consume pending details
	err := sb.spaceIndex.UpdatePendingLocalDetails(sb.Id(), func(pending *domain.Details) (*domain.Details, error) {
		if pending.Len() > 0 {
			hasPendingLocalDetails = true
		}
		details = details.Merge(pending)
		return nil, nil
	})
	if err != nil {
		log.With("objectID", sb.Id()).
			With("sbType", sb.Type()).Errorf("failed to update pending details: %v", err)
	}
	return details, hasPendingLocalDetails
}

func (sb *smartBlock) getCreationInfo() (creatorObjectId string, treeCreatedDate int64, err error) {
	creatorObjectId, treeCreatedDate, err = sb.source.GetCreationInfo()
	if err != nil {
		return
	}

	return creatorObjectId, treeCreatedDate, nil
}

func (sb *smartBlock) injectCreationInfo(s *state.State) error {
	if sb.Type() == smartblock.SmartBlockTypeProfilePage {
		// todo: for the shared spaces we need to change this for sophisticated logic
		creatorIdentityObjectId, _, err := sb.getCreationInfo()
		if err != nil {
			return err
		}

		if creatorIdentityObjectId != "" {
			s.SetDetail(bundle.RelationKeyProfileOwnerIdentity, domain.String(creatorIdentityObjectId))
		}
	} else {
		// make sure we don't have this relation for other objects
		s.RemoveLocalDetail(bundle.RelationKeyProfileOwnerIdentity)
	}

	if s.LocalDetails().GetString(bundle.RelationKeyCreator) != "" && s.LocalDetails().GetInt64(bundle.RelationKeyCreatedDate) != 0 {
		return nil
	}

	creatorIdentityObjectId, treeCreatedDate, err := sb.getCreationInfo()
	if err != nil {
		return err
	}

	if creatorIdentityObjectId != "" {
		s.SetDetail(bundle.RelationKeyCreator, domain.String(creatorIdentityObjectId))
	} else {
		// For derived objects we set current identity
		s.SetDetail(bundle.RelationKeyCreator, domain.String(sb.currentParticipantId))
	}

	if originalCreated := s.OriginalCreatedTimestamp(); originalCreated > 0 {
		// means we have imported object, so we need to set original created date
		s.SetDetail(bundle.RelationKeyCreatedDate, domain.Int64(originalCreated))
		// Only set AddedDate once because we have a side effect with treeCreatedDate:
		// - When we import object, treeCreateDate is set to time.Now()
		// - But after push it is changed to original modified date
		// - So after account recovery we will get treeCreateDate = original modified date, which is not equal to AddedDate
		if s.Details().GetInt64(bundle.RelationKeyAddedDate) == 0 {
			s.SetDetail(bundle.RelationKeyAddedDate, domain.Int64(treeCreatedDate))
		}
	} else {
		s.SetDetail(bundle.RelationKeyCreatedDate, domain.Int64(treeCreatedDate))
	}

	return nil
}

// injectDerivedDetails injects the local data
func (sb *smartBlock) injectDerivedDetails(s *state.State, spaceID string, sbt smartblock.SmartBlockType) {
	// TODO Pick from source
	id := s.RootId()
	if id != "" {
		s.SetDetail(bundle.RelationKeyId, domain.String(id))
	}

	if v, ok := s.Details().TryInt64(bundle.RelationKeyFileBackupStatus); ok {
		status := filesyncstatus.Status(v)
		// Clients expect syncstatus constants in this relation
		s.SetDetail(bundle.RelationKeyFileSyncStatus, domain.Int64(status.ToSyncStatus()))
	}

	if info := s.GetFileInfo(); info.FileId != "" {
		err := sb.fileStore.AddFileKeys(domain.FileEncryptionKeys{
			FileId:         info.FileId,
			EncryptionKeys: info.EncryptionKeys,
		})
		if err != nil {
			log.Errorf("failed to store file keys: %v", err)
		}
	}

	if spaceID != "" {
		s.SetDetail(bundle.RelationKeySpaceId, domain.String(spaceID))
	} else {
		log.Errorf("InjectDerivedDetails: failed to set space id for %s: no space id provided, but in details: %s", id, s.LocalDetails().GetString(bundle.RelationKeySpaceId))
	}
	if ot := s.ObjectTypeKey(); ot != "" {
		typeID, err := sb.space.GetTypeIdByKey(context.Background(), ot)
		if err != nil {
			log.Errorf("failed to get type id for %s: %v", ot, err)
		}

		s.SetDetail(bundle.RelationKeyType, domain.String(typeID))
	}

	if uki := s.UniqueKeyInternal(); uki != "" {
		// todo: remove this hack after spaceService refactored to include marketplace virtual space
		if sbt == smartblock.SmartBlockTypeBundledObjectType {
			sbt = smartblock.SmartBlockTypeObjectType
		} else if sbt == smartblock.SmartBlockTypeBundledRelation {
			sbt = smartblock.SmartBlockTypeRelation
		}

		uk, err := domain.NewUniqueKey(sbt, uki)
		if err != nil {
			log.Errorf("failed to get unique key for %s: %v", uki, err)
		} else {
			s.SetDetail(bundle.RelationKeyUniqueKey, domain.String(uk.Marshal()))
		}
	}

	err := sb.deriveChatId(s)
	if err != nil {
		log.With("objectId", sb.Id()).Errorf("can't derive chat id: %v", err)
	}

	sb.setRestrictionsDetail(s)

	snippet := s.Snippet()
	if snippet != "" || s.LocalDetails() != nil {
		s.SetDetail(bundle.RelationKeySnippet, domain.String(snippet))
	}

	// Set isDeleted relation only if isUninstalled is present in details
	if isUninstalled, ok := s.Details().TryBool(bundle.RelationKeyIsUninstalled); ok {
		var isDeleted bool
		if isUninstalled {
			isDeleted = true
		}
		s.SetDetail(bundle.RelationKeyIsDeleted, domain.Bool(isDeleted))
	}

	sb.injectResolvedLayout(s)
	sb.injectLinksDetails(s)
	sb.injectMentions(s)
	sb.updateBackLinks(s)
}

func (sb *smartBlock) deriveChatId(s *state.State) error {
	hasChat := s.Details().GetBool(bundle.RelationKeyHasChat)
	if hasChat {
		chatUk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeChatDerivedObject, sb.Id())
		if err != nil {
			return err
		}

		chatId, err := sb.space.DeriveObjectID(context.Background(), chatUk)
		if err != nil {
			return err
		}
		s.SetDetail(bundle.RelationKeyChatId, domain.String(chatId))
	}
	return nil
}

func (sb *smartBlock) injectResolvedLayout(s *state.State) {
	if s.Details() == nil && s.LocalDetails() == nil {
		return
	}
	rawValue := s.Details().Get(bundle.RelationKeyLayout)
	if rawValue.Ok() {
		s.SetDetail(bundle.RelationKeyResolvedLayout, rawValue)
		return
	}

	typeObjectId := s.LocalDetails().GetString(bundle.RelationKeyType)

	if s.ObjectTypeKey() == bundle.TypeKeyTemplate {
		// resolvedLayout for templates should be derived from target type
		typeObjectId = s.Details().GetString(bundle.RelationKeyTargetObjectType)
	}

	if typeObjectId == "" {
		if currentValue := s.LocalDetails().Get(bundle.RelationKeyResolvedLayout); currentValue.Ok() {
			return
		}
		log.Errorf("failed to find id of object type. Falling back to basic layout")
		s.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_basic)))
		return
	}

	currentValue := s.LocalDetails().Get(bundle.RelationKeyResolvedLayout)

	typeDetails, found := sb.lastDepDetails[typeObjectId]
	if found {
		rawValue = typeDetails.Get(bundle.RelationKeyRecommendedLayout)
	} else {
		records, err := sb.objectStore.SpaceIndex(sb.SpaceID()).QueryByIds([]string{typeObjectId})
		if err != nil || len(records) != 1 {
			log.Errorf("failed to query object %s: %v", typeObjectId, err)
			if currentValue.Ok() {
				return
			}
			s.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_basic)))
			return
		}
		rawValue = records[0].Details.Get(bundle.RelationKeyRecommendedLayout)
	}

	if !rawValue.Ok() {
		if currentValue.Ok() {
			return
		}
		log.Errorf("failed to get recommended layout from details of type. Fallback to basic layout")
		s.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_basic)))
		return
	}

	s.SetDetail(bundle.RelationKeyResolvedLayout, rawValue)
}

// changeResolvedLayoutForObjects changes resolvedLayout for object of this type and deletes Layout relation
func (sb *smartBlock) changeResolvedLayoutForObjects(msgs []simple.EventMessage, deleteLayoutRelation bool) error {
	if sb.Type() != smartblock.SmartBlockTypeObjectType {
		return nil
	}

	layout, found := getLayoutFromMessages(msgs)
	if !found {
		// recommendedLayout was not changed
		return nil
	}

	// nolint:gosec
	if !isLayoutChangeApplicable(model.ObjectTypeLayout(layout)) {
		// if layout change is not applicable, then it is init of some system type
		return nil
	}

	index := sb.objectStore.SpaceIndex(sb.SpaceID())
	records, err := index.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(sb.Id()),
		},
	}})
	if err != nil {
		return fmt.Errorf("failed to get objects of single type: %w", err)
	}

	templates, err := index.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyTargetObjectType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(sb.Id()),
		},
	}})
	if err != nil {
		return fmt.Errorf("failed to get templates with this target type: %w", err)
	}

	var resultErr error
	for _, record := range append(records, templates...) {
		id := record.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}
		if deleteLayoutRelation && record.Details.Has(bundle.RelationKeyLayout) {
			// we should delete layout from object, that's why we apply changes even if object is not in cache
			err = sb.space.Do(id, func(b SmartBlock) error {
				st := b.NewState()
				st.RemoveDetail(bundle.RelationKeyLayout)
				st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(layout))
				return b.Apply(st)
			})
			if err != nil {
				resultErr = errors.Join(resultErr, err)
			}
			continue
		}

		if record.Details.GetInt64(bundle.RelationKeyResolvedLayout) == layout {
			// relevant layout is already set, skipping
			continue
		}

		err = sb.space.DoLockedIfNotExists(id, func() error {
			return index.ModifyObjectDetails(id, func(details *domain.Details) (*domain.Details, bool, error) {
				if details == nil {
					return nil, false, nil
				}
				if details.GetInt64(bundle.RelationKeyResolvedLayout) == layout {
					return nil, false, nil
				}
				details.Set(bundle.RelationKeyResolvedLayout, domain.Int64(layout))
				return details, true, nil
			})
		})

		if err == nil {
			continue
		}

		if !errors.Is(err, ocache.ErrExists) {
			resultErr = errors.Join(resultErr, err)
			continue
		}

		err = sb.space.Do(id, func(b SmartBlock) error {
			st := b.NewState()
			st.SetDetail(bundle.RelationKeyResolvedLayout, domain.Int64(layout))
			return b.Apply(st, KeepInternalFlags, NotPushChanges)
		})
		if err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}

	if resultErr != nil {
		return fmt.Errorf("failed to change layout for objects: %w", resultErr)
	}
	return nil
}

func getLayoutFromMessages(msgs []simple.EventMessage) (layout int64, found bool) {
	for _, ev := range msgs {
		if amend := ev.Msg.GetObjectDetailsAmend(); amend != nil {
			for _, detail := range amend.Details {
				if detail.Key == bundle.RelationKeyRecommendedLayout.String() {
					return int64(detail.Value.GetNumberValue()), true
				}
			}
		}
	}
	return 0, false
}

func isLayoutChangeApplicable(layout model.ObjectTypeLayout) bool {
	return slices.Contains([]model.ObjectTypeLayout{
		model.ObjectType_basic,
		model.ObjectType_todo,
		model.ObjectType_profile,
		model.ObjectType_note,
		model.ObjectType_collection,
	}, layout)
}
