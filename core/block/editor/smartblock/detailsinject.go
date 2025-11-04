package smartblock

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var layoutPerSmartBlockType = map[smartblock.SmartBlockType]model.ObjectTypeLayout{
	smartblock.SmartBlockTypeRelation:             model.ObjectType_relation,
	smartblock.SmartBlockTypeBundledRelation:      model.ObjectType_relation,
	smartblock.SmartBlockTypeObjectType:           model.ObjectType_objectType,
	smartblock.SmartBlockTypeBundledObjectType:    model.ObjectType_objectType,
	smartblock.SmartBlockTypeRelationOption:       model.ObjectType_relationOption,
	smartblock.SmartBlockTypeSpaceView:            model.ObjectType_spaceView,
	smartblock.SmartBlockTypeParticipant:          model.ObjectType_participant,
	smartblock.SmartBlockTypeFile:                 model.ObjectType_file, // deprecated
	smartblock.SmartBlockTypeDate:                 model.ObjectType_date,
	smartblock.SmartBlockTypeChatDerivedObject:    model.ObjectType_chatDerived,
	smartblock.SmartBlockTypeChatObjectDeprecated: model.ObjectType_chatDeprecated, // deprecated
	smartblock.SmartBlockTypeWidget:               model.ObjectType_dashboard,
	smartblock.SmartBlockTypeWorkspace:            model.ObjectType_dashboard,
	smartblock.SmartBlockTypeArchive:              model.ObjectType_dashboard,
	smartblock.SmartBlockTypeHome:                 model.ObjectType_dashboard,
	smartblock.SmartBlockTypeAccountObject:        model.ObjectType_profile,
	smartblock.SmartBlockTypeAnytypeProfile:       model.ObjectType_profile,
	smartblock.SmartBlockTypeIdentity:             model.ObjectType_profile,
	smartblock.SmartBlockTypeProfilePage:          model.ObjectType_profile,
	smartblock.SmartBlockTypeAccountOld:           model.ObjectType_profile, // deprecated
	smartblock.SmartBlockTypeMissingObject:        model.ObjectType_missingObject,
	smartblock.SmartBlockTypeNotificationObject:   model.ObjectType_notification,
	smartblock.SmartBlockTypeDevicesObject:        model.ObjectType_devices,
}

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
	localDetailsFromStore.Delete(bundle.RelationKeyResolvedLayout)
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
		if pending.Len() > 1 { // more than just id
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
		err := sb.objectStore.AddFileKeys(domain.FileEncryptionKeys{
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

// resolveLayout adds resolvedLayout to local details of object. Priority:
// layout restricted by sbType > layout > recommendedLayout from type > current resolvedLayout > basic (fallback)
// resolveLayout also converts object from Note, i.e. adds Name and Title to state
func (sb *smartBlock) resolveLayout(s *state.State) {
	if s.Details() == nil && s.LocalDetails() == nil {
		return
	}
	var (
		layoutValue  = s.Details().Get(bundle.RelationKeyLayout)
		currentValue = s.LocalDetails().Get(bundle.RelationKeyResolvedLayout)
		newValue     domain.Value

		sbTypeLayoutValue, hasStrictLayout = layoutPerSmartBlockType[sb.Type()]
	)

	if hasStrictLayout {
		s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(sbTypeLayoutValue)))
		return
	}

	if !currentValue.Ok() && layoutValue.Ok() {
		// we don't have resolvedLayout in local details, but we have layout
		currentValue = layoutValue
	}

	typeDetails, err := sb.getTypeDetails(s)
	valueInType := typeDetails.Get(bundle.RelationKeyRecommendedLayout)
	if layoutValue.Ok() {
		newValue = layoutValue
	} else if valueInType.Ok() {
		newValue = valueInType
	} else if currentValue.Ok() {
		newValue = currentValue
	} else {
		log.Warnf("failed to get recommended layout from details of type: %v. Fallback to basic layout", err)
		newValue = sb.getFallbackLayoutValue(s)
	}

	if newValue.Ok() {
		s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, newValue)
	}

	convertLayoutBlocks(s, currentValue, newValue)
}

func (sb *smartBlock) getFallbackLayoutValue(s *state.State) domain.Value {
	if len(s.ObjectTypeKeys()) > 0 {
		typeKey := s.ObjectTypeKeys()[len(s.ObjectTypeKeys())-1]
		if bt, err := bundle.GetType(typeKey); err == nil && typeKey != bundle.TypeKeyTemplate {
			return domain.Int64(int64(bt.Layout))
		}
	}

	if sb.Type() == smartblock.SmartBlockTypeFileObject {
		// for file object we use file layout
		return domain.Int64(int64(model.ObjectType_file))
	}

	if s.Exists(state.TitleBlockID) {
		return domain.Int64(int64(model.ObjectType_basic))
	}
	return domain.Int64(int64(model.ObjectType_note))
}

func convertLayoutBlocks(st *state.State, oldLayout, newLayout domain.Value) {
	if !newLayout.Ok() {
		return
	}
	if oldLayout.Equal(newLayout) {
		return
	}
	if newLayout.Int64() != int64(model.ObjectType_note) {
		if st.Exists(state.TitleBlockID) {
			return
		}
		log.With("objectId", st.RootId()).Infof("convert layout: %s -> %s", oldLayout, newLayout)
		template.InitTemplate(st, template.WithNameFromFirstBlock, template.WithTitle)
	} else if newLayout.Int64() == int64(model.ObjectType_note) {
		if !st.Exists(state.TitleBlockID) {
			return
		}

		log.With("objectId", st.RootId()).Infof("convert layout: %s -> %s", oldLayout, newLayout)
		template.InitTemplate(st,
			template.WithNameToFirstBlock,
			template.WithNoTitle,
			template.WithNoDescription,
		)
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
		return nil, fmt.Errorf("failed to query object %s: %v", typeObjectId, err)
	}
	return records[0].Details, nil
}

func (sb *smartBlock) setRestrictionsDetail(s *state.State) {
	currentRestrictions := restriction.NewObjectRestrictionsFromValue(s.LocalDetails().Get(bundle.RelationKeyRestrictions))
	if currentRestrictions.Equal(sb.Restrictions().Object) {
		return
	}

	s.SetLocalDetail(bundle.RelationKeyRestrictions, sb.Restrictions().Object.ToValue())

	if sb.Restrictions().Object.Check(model.Restrictions_Details) != nil &&
		sb.Restrictions().Object.Check(model.Restrictions_Blocks) != nil {
		s.SetDetailAndBundledRelation(bundle.RelationKeyIsReadonly, domain.Bool(true))
	} else if s.LocalDetails().GetBool(bundle.RelationKeyIsReadonly) {
		s.SetDetailAndBundledRelation(bundle.RelationKeyIsReadonly, domain.Bool(false))
	}
}
