package smartblock

import (
	"context"
	"fmt"
	"slices"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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
			s.SetDetailAndBundledRelation(bundle.RelationKeyProfileOwnerIdentity, domain.String(creatorIdentityObjectId))
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
		s.SetDetailAndBundledRelation(bundle.RelationKeyCreator, domain.String(creatorIdentityObjectId))
	} else {
		// For derived objects we set current identity
		s.SetDetailAndBundledRelation(bundle.RelationKeyCreator, domain.String(sb.currentParticipantId))
	}

	if originalCreated := s.OriginalCreatedTimestamp(); originalCreated > 0 {
		// means we have imported object, so we need to set original created date
		s.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, domain.Int64(originalCreated))
		// Only set AddedDate once because we have a side effect with treeCreatedDate:
		// - When we import object, treeCreateDate is set to time.Now()
		// - But after push it is changed to original modified date
		// - So after account recovery we will get treeCreateDate = original modified date, which is not equal to AddedDate
		if s.Details().GetInt64(bundle.RelationKeyAddedDate) == 0 {
			s.SetDetailAndBundledRelation(bundle.RelationKeyAddedDate, domain.Int64(treeCreatedDate))
		}
	} else {
		s.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, domain.Int64(treeCreatedDate))
	}

	return nil
}

// injectDerivedDetails injects the local data
func (sb *smartBlock) injectDerivedDetails(s *state.State, spaceID string, sbt smartblock.SmartBlockType) {
	// TODO Pick from source
	id := s.RootId()
	if id != "" {
		s.SetDetailAndBundledRelation(bundle.RelationKeyId, domain.String(id))
	}

	if v, ok := s.Details().TryInt64(bundle.RelationKeyFileBackupStatus); ok {
		status := filesyncstatus.Status(v)
		// Clients expect syncstatus constants in this relation
		s.SetDetailAndBundledRelation(bundle.RelationKeyFileSyncStatus, domain.Int64(status.ToSyncStatus()))
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
		s.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, domain.String(spaceID))
	} else {
		log.Errorf("InjectDerivedDetails: failed to set space id for %s: no space id provided, but in details: %s", id, s.LocalDetails().GetString(bundle.RelationKeySpaceId))
	}
	if ot := s.ObjectTypeKey(); ot != "" {
		typeID, err := sb.space.GetTypeIdByKey(context.Background(), ot)
		if err != nil {
			log.Errorf("failed to get type id for %s: %v", ot, err)
		}

		s.SetDetailAndBundledRelation(bundle.RelationKeyType, domain.String(typeID))
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
			s.SetDetailAndBundledRelation(bundle.RelationKeyUniqueKey, domain.String(uk.Marshal()))
		}
	}

	err := sb.deriveChatId(s)
	if err != nil {
		log.With("objectId", sb.Id()).Errorf("can't derive chat id: %v", err)
	}

	sb.setRestrictionsDetail(s)

	snippet := s.Snippet()
	if snippet != "" || s.LocalDetails() != nil {
		s.SetDetailAndBundledRelation(bundle.RelationKeySnippet, domain.String(snippet))
	}

	// Set isDeleted relation only if isUninstalled is present in details
	if isUninstalled, ok := s.Details().TryBool(bundle.RelationKeyIsUninstalled); ok {
		var isDeleted bool
		if isUninstalled {
			isDeleted = true
		}
		s.SetDetailAndBundledRelation(bundle.RelationKeyIsDeleted, domain.Bool(isDeleted))
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
		s.SetDetailAndBundledRelation(bundle.RelationKeyChatId, domain.String(chatId))
	}
	return nil
}

// injectResolvedLayout adds resolvedLayout to local details of object. It analyzes details of object type
// - if isEnforcedLayout == true, resolvedLayout must be the same as recommendedLayout of object type
// - if isEnforcedLayout == false and object has layout detail, resolvedLayout should be the same as layout value
// - if isEnforcedLayout == false and object has no layout detail set, resolvedLayout should be the same as recommendedLayout of object type
// - if object is a template, isEnforcedLayout flag should be ignored and priority will be 1. layout 2. recommendedLayout of target type
// - fallback is current resolvedLayout value. If it is not ok, then basic
func (sb *smartBlock) injectResolvedLayout(s *state.State) {
	if s.Details() == nil && s.LocalDetails() == nil {
		return
	}

	var (
		layoutValue  = s.Details().Get(bundle.RelationKeyLayout)
		currentValue = s.LocalDetails().Get(bundle.RelationKeyResolvedLayout)
	)

	typeDetails, err := sb.getTypeDetails(s)
	if err != nil {
		if layoutValue.Ok() {
			s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, layoutValue)
		} else if !currentValue.Ok() {
			log.Errorf("failed to get recommended layout from details of type: %v. Fallback to basic layout", err)
			s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_basic)))
		}
		return
	}

	var (
		valueInType      = typeDetails.Get(bundle.RelationKeyRecommendedLayout)
		isEnforcedLayout = typeDetails.GetBool(bundle.RelationKeyForceLayoutFromType)
	)

	if s.ObjectTypeKey() == bundle.TypeKeyTemplate || !isEnforcedLayout {
		if layoutValue.Ok() {
			s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, layoutValue)
		} else if valueInType.Ok() {
			s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, valueInType)
		} else if !currentValue.Ok() {
			log.Errorf("failed to get recommended layout from details of type: %v. Fallback to basic layout", err)
			s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_basic)))
		}
		return
	}

	if valueInType.Ok() {
		s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, valueInType)
	} else if layoutValue.Ok() {
		s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, layoutValue)
	} else if !currentValue.Ok() {
		log.Errorf("failed to get recommended layout from details of type: %v. Fallback to basic layout", err)
		s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_basic)))
	}

	featuredRelations := make([]string, 0)
	if slices.Contains(s.Details().GetStringList(bundle.RelationKeyFeaturedRelations), bundle.RelationKeyDescription.String()) {
		featuredRelations = []string{bundle.RelationKeyDescription.String()}
	}

	s.Details().Delete(bundle.RelationKeyLayout)
	s.Details().Delete(bundle.RelationKeyLayoutAlign)
	s.Details().SetStringList(bundle.RelationKeyFeaturedRelations, featuredRelations)

	// we delete layout settings details both from parent and child state to avoid changes generation
	if parent := s.ParentState(); parent != nil {
		parent.Details().Delete(bundle.RelationKeyLayout)
		parent.Details().Delete(bundle.RelationKeyLayoutAlign)
		parent.Details().SetStringList(bundle.RelationKeyFeaturedRelations, featuredRelations)
	}
}

func (sb *smartBlock) getTypeDetails(s *state.State) (*domain.Details, error) {
	typeObjectId := s.LocalDetails().GetString(bundle.RelationKeyType)

	if s.ObjectTypeKey() == bundle.TypeKeyTemplate {
		// resolvedLayout for templates should be derived from target type
		typeObjectId = s.Details().GetString(bundle.RelationKeyTargetObjectType)
	}

	if typeObjectId == "" {
		return nil, fmt.Errorf("failed to find id of object type")
	}

	typeDetails, found := sb.lastDepDetails[typeObjectId]
	if found {
		return typeDetails, nil
	}

	records, err := sb.objectStore.SpaceIndex(sb.SpaceID()).QueryByIds([]string{typeObjectId})
	if err != nil || len(records) != 1 {
		return nil, fmt.Errorf("failed to query object %s: %w", typeObjectId, err)
	}
	return records[0].Details, nil
}
