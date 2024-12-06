package smartblock

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

	localDetailsFromStore := pbtypes.StructFilterKeys(details, keys)

	s.InjectLocalDetails(localDetailsFromStore)
	if p := s.ParentState(); p != nil && !hasPendingLocalDetails {
		// inject for both current and parent state
		p.InjectLocalDetails(localDetailsFromStore)
	}

	err = sb.injectCreationInfo(s)
	if err != nil {
		log.With("objectID", sb.Id()).With("sbtype", sb.Type().String()).Errorf("failed to inject creation info: %s", err.Error())
	}
	return nil
}

func (sb *smartBlock) getDetailsFromStore() (*types.Struct, error) {
	storedDetails, err := sb.spaceIndex.GetDetails(sb.Id())
	if err != nil || storedDetails == nil {
		return nil, err
	}
	return pbtypes.CopyStruct(storedDetails.GetDetails(), true), nil
}

func (sb *smartBlock) appendPendingDetails(details *types.Struct) (resultDetails *types.Struct, hasPendingLocalDetails bool) {
	// Consume pending details
	err := sb.spaceIndex.UpdatePendingLocalDetails(sb.Id(), func(pending *types.Struct) (*types.Struct, error) {
		if len(pending.GetFields()) > 0 {
			hasPendingLocalDetails = true
		}
		details = pbtypes.StructMerge(details, pending, false)
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
			s.SetDetailAndBundledRelation(bundle.RelationKeyProfileOwnerIdentity, pbtypes.String(creatorIdentityObjectId))
		}
	} else {
		// make sure we don't have this relation for other objects
		s.RemoveLocalDetail(bundle.RelationKeyProfileOwnerIdentity.String())
	}

	if pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyCreator.String()) != "" && pbtypes.GetInt64(s.LocalDetails(), bundle.RelationKeyCreatedDate.String()) != 0 {
		return nil
	}

	creatorIdentityObjectId, treeCreatedDate, err := sb.getCreationInfo()
	if err != nil {
		return err
	}

	if creatorIdentityObjectId != "" {
		s.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(creatorIdentityObjectId))
	} else {
		// For derived objects we set current identity
		s.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(sb.currentParticipantId))
	}

	if originalCreated := s.OriginalCreatedTimestamp(); originalCreated > 0 {
		// means we have imported object, so we need to set original created date
		s.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Float64(float64(originalCreated)))
		// Only set AddedDate once because we have a side effect with treeCreatedDate:
		// - When we import object, treeCreateDate is set to time.Now()
		// - But after push it is changed to original modified date
		// - So after account recovery we will get treeCreateDate = original modified date, which is not equal to AddedDate
		if pbtypes.GetInt64(s.Details(), bundle.RelationKeyAddedDate.String()) == 0 {
			s.SetDetailAndBundledRelation(bundle.RelationKeyAddedDate, pbtypes.Float64(float64(treeCreatedDate)))
		}
	} else {
		s.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Float64(float64(treeCreatedDate)))
	}

	return nil
}

// injectDerivedDetails injects the local data
func (sb *smartBlock) injectDerivedDetails(s *state.State, spaceID string, sbt smartblock.SmartBlockType) {
	id := s.RootId()
	if id != "" {
		s.SetDetailAndBundledRelation(bundle.RelationKeyId, pbtypes.String(id))
	}

	if v, ok := s.Details().GetFields()[bundle.RelationKeyFileBackupStatus.String()]; ok {
		status := filesyncstatus.Status(v.GetNumberValue())
		// Clients expect syncstatus constants in this relation
		s.SetDetailAndBundledRelation(bundle.RelationKeyFileSyncStatus, pbtypes.Int64(int64(status.ToSyncStatus())))
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
		s.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, pbtypes.String(spaceID))
	} else {
		log.Errorf("InjectDerivedDetails: failed to set space id for %s: no space id provided, but in details: %s", id, pbtypes.GetString(s.LocalDetails(), bundle.RelationKeySpaceId.String()))
	}
	if ot := s.ObjectTypeKey(); ot != "" {
		typeID, err := sb.space.GetTypeIdByKey(context.Background(), ot)
		if err != nil {
			log.Errorf("failed to get type id for %s: %v", ot, err)
		}

		s.SetDetailAndBundledRelation(bundle.RelationKeyType, pbtypes.String(typeID))
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
			s.SetDetailAndBundledRelation(bundle.RelationKeyUniqueKey, pbtypes.String(uk.Marshal()))
		}
	}

	err := sb.deriveChatId(s)
	if err != nil {
		log.With("objectId", sb.Id()).Errorf("can't derive chat id: %v", err)
	}

	sb.setRestrictionsDetail(s)

	snippet := s.Snippet()
	if snippet != "" || s.LocalDetails() != nil {
		s.SetDetailAndBundledRelation(bundle.RelationKeySnippet, pbtypes.String(snippet))
	}

	// Set isDeleted relation only if isUninstalled is present in details
	if isUninstalled := s.Details().GetFields()[bundle.RelationKeyIsUninstalled.String()]; isUninstalled != nil {
		var isDeleted bool
		if isUninstalled.GetBoolValue() {
			isDeleted = true
		}
		s.SetDetailAndBundledRelation(bundle.RelationKeyIsDeleted, pbtypes.Bool(isDeleted))
	}

	sb.injectLinksDetails(s)
	sb.injectMentions(s)
	sb.updateBackLinks(s)

	sb.injectResolvedLayout(s)
}

func (sb *smartBlock) deriveChatId(s *state.State) error {
	hasChat := pbtypes.GetBool(s.Details(), bundle.RelationKeyHasChat.String())
	if hasChat {
		chatUk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeChatDerivedObject, sb.Id())
		if err != nil {
			return err
		}

		chatId, err := sb.space.DeriveObjectID(context.Background(), chatUk)
		if err != nil {
			return err
		}
		s.SetDetailAndBundledRelation(bundle.RelationKeyChatId, pbtypes.String(chatId))
	}
	return nil
}

func (sb *smartBlock) injectResolvedLayout(s *state.State) {
	if s.Details() == nil {
		return
	}
	rawValue := s.Details().Fields[bundle.RelationKeyLayout.String()]
	if rawValue != nil {
		s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, rawValue)
		return
	}

	typeObjectId := pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyType.String())

	// I have no clue when lastDepDetails are filled, so maybe we need to call metaListener directly
	typeDetails, found := sb.lastDepDetails[typeObjectId]
	if !found {
		log.Errorf("failed to find details of type in smartblock dependencies")
		s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, pbtypes.Int64(int64(model.ObjectType_basic)))
		return
	}

	rawValue = typeDetails.Details.Fields[bundle.RelationKeyRecommendedLayout.String()]
	if rawValue == nil {
		log.Errorf("failed to get recommended layout from details of type")
		s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, pbtypes.Int64(int64(model.ObjectType_basic)))
		return
	}

	s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, rawValue)
}
