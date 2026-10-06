package smartblock

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
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
	smartblock.SmartBlockTypeDiscussionObject:     model.ObjectType_discussion,
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
//
// It never touches blocks or the name: it runs on every load, apply and remote change, so a
// conversion here is written by every device that loads the object, each from its own view of
// the type. When a type's recommended layout changed, that made hundreds of members of a shared
// space rewrite the same objects concurrently, back and forth. Blocks are brought in line with the
// layout explicitly instead - see ConvertLayoutBlocks, and convertNewObjectLayoutBlocks for the
// creation of an object.
//
// It returns the previous and the new resolved layout, and whether the new one is known rather
// than guessed.
func (sb *smartBlock) resolveLayout(s *state.State) (currentValue, newValue domain.Value, layoutIsKnown bool) {
	if s.Details() == nil && s.LocalDetails() == nil {
		return
	}
	var (
		layoutValue = s.Details().Get(bundle.RelationKeyLayout)

		sbTypeLayoutValue, hasStrictLayout = layoutPerSmartBlockType[sb.Type()]
	)
	currentValue = s.LocalDetails().Get(bundle.RelationKeyResolvedLayout)

	if hasStrictLayout {
		s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(sbTypeLayoutValue)))
		return currentValue, domain.Value{}, false
	}

	if !currentValue.Ok() && layoutValue.Ok() {
		// we don't have resolvedLayout in local details, but we have layout
		currentValue = layoutValue
	}

	typeDetails, err := sb.getTypeDetails(s)
	valueInType := typeDetails.Get(bundle.RelationKeyRecommendedLayout)
	layoutIsKnown = true
	if layoutValue.Ok() {
		newValue = layoutValue
	} else if valueInType.Ok() {
		newValue = valueInType
	} else if currentValue.Ok() {
		newValue = currentValue
	} else {
		log.Warnf("failed to get recommended layout from details of type: %v. Guessing the layout", err)
		newValue, layoutIsKnown = sb.getFallbackLayoutValue(s)
	}

	if newValue.Ok() {
		s.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, newValue)
	}
	return currentValue, newValue, layoutIsKnown
}

// ConvertLayoutBlocks brings the blocks of the object in line with its layout, see
// ConvertLayoutBlocksTo. It reports whether s was changed.
//
// Call it only on an explicit user action, such as opening the object: the change it makes is
// derived from this device's view of the object type, and must not be repeated by every device
// that merely loads the object. Only an authoritative layout is used - the layout detail or the
// type's recommended layout - never a guess, a bundled type's default or the stored
// resolvedLayout, any of which can disagree with what resolveLayout keeps for the object.
func (sb *smartBlock) ConvertLayoutBlocks(s *state.State) bool {
	if _, hasStrictLayout := layoutPerSmartBlockType[sb.Type()]; hasStrictLayout {
		return false
	}
	layout, ok := sb.knownLayout(s)
	if !ok {
		return false
	}
	return ConvertLayoutBlocksTo(s, layout)
}

func (sb *smartBlock) knownLayout(s *state.State) (model.ObjectTypeLayout, bool) {
	if v, ok := s.Details().TryInt64(bundle.RelationKeyLayout); ok {
		return model.ObjectTypeLayout(v), true // nolint:gosec
	}
	typeDetails, _ := sb.typeDetailsById(sb.currentTypeId(s))
	if v, ok := typeDetails.TryInt64(bundle.RelationKeyRecommendedLayout); ok {
		return model.ObjectTypeLayout(v), true // nolint:gosec
	}
	return 0, false
}

// currentTypeId is the id of the type the layout of s follows, taken from s itself: the type
// detail in local details is only refreshed on Apply, so within an edit that changes the type
// it still names the old one
func (sb *smartBlock) currentTypeId(s *state.State) string {
	key := s.ObjectTypeKey()
	if key == bundle.TypeKeyTemplate {
		return s.Details().GetString(bundle.RelationKeyTargetObjectType)
	}
	if key != "" {
		if id, err := sb.space.GetTypeIdByKey(context.Background(), key); err == nil && id != "" {
			return id
		}
	}
	return s.LocalDetails().GetString(bundle.RelationKeyType)
}

// getFallbackLayoutValue is the last resort when neither the object nor its type tells us
// the layout. layoutIsKnown reports whether the value is actually derived from something
// authoritative (a bundled type, the smartblock type) rather than merely guessed.
func (sb *smartBlock) getFallbackLayoutValue(s *state.State) (value domain.Value, layoutIsKnown bool) {
	if len(s.ObjectTypeKeys()) > 0 {
		typeKey := s.ObjectTypeKeys()[len(s.ObjectTypeKeys())-1]
		if bt, err := bundle.GetType(typeKey); err == nil && typeKey != bundle.TypeKeyTemplate {
			return domain.Int64(int64(bt.Layout)), true
		}
	}

	if sb.Type() == smartblock.SmartBlockTypeFileObject {
		// for file object we use file layout
		return domain.Int64(int64(model.ObjectType_file)), true
	}

	// Basic is the layout new types get by default. Guessing note here used to be based on
	// the absence of a title block, which is wrong for objects whose title lives in the name
	// detail (every imported one), and note is the single layout whose conversion deletes
	// the name detail - so a wrong guess silently lost the object's name.
	return domain.Int64(int64(model.ObjectType_basic)), false
}

// LayoutSourceChanged reports whether s changes what the object's layout is taken from, compared
// to its parent state: its own layout detail, or the target type of a template. A write that does
// is an explicit layout change, and its blocks should follow it - see ConvertLayoutBlocks.
func LayoutSourceChanged(s *state.State) bool {
	parent := s.ParentState()
	if parent == nil {
		return false
	}
	for _, key := range []domain.RelationKey{bundle.RelationKeyLayout, bundle.RelationKeyTargetObjectType} {
		if !s.Details().Get(key).Equal(parent.Details().Get(key)) {
			return true
		}
	}
	return false
}

// ConvertLayoutBlocksTo converts the blocks of a page-layout object between the note form (no
// title, the name lives in the first text block) and the titled form of the other page layouts,
// whichever layout says. The decision is taken from the blocks alone, so the conversion is
// idempotent: once the blocks match the layout it is a no-op. It reports whether s was changed.
func ConvertLayoutBlocksTo(st *state.State, layout model.ObjectTypeLayout) bool {
	if !converter.IsPageLayout(layout) {
		return false
	}
	// a title block can be present in the state and yet hang from no parent, see WithTitle
	hasTitle := st.Exists(state.TitleBlockID) && st.PickParentOf(state.TitleBlockID) != nil
	if layout == model.ObjectType_note {
		if !hasTitle {
			return false
		}
		log.With("objectId", st.RootId()).Infof("convert layout blocks to %s", layout)
		template.InitTemplate(st,
			template.WithNameToFirstBlock,
			template.WithNoTitle,
			template.WithNoDescription,
		)
		return true
	}
	if hasTitle {
		return false
	}
	log.With("objectId", st.RootId()).Infof("convert layout blocks to %s", layout)
	templates := []template.StateTransformer{template.WithNameFromFirstBlock, template.WithTitle}
	if st.Details().GetString(bundle.RelationKeyDescription) != "" {
		templates = append(templates, template.WithDescription)
	}
	template.InitTemplate(st, templates...)
	return true
}

func (sb *smartBlock) getTypeDetails(s *state.State) (*domain.Details, error) {
	typeObjectId := s.LocalDetails().GetString(bundle.RelationKeyType)

	if s.ObjectTypeKey() == bundle.TypeKeyTemplate {
		// resolvedLayout for templates should be derived from target type
		typeObjectId = s.Details().GetString(bundle.RelationKeyTargetObjectType)
	}

	return sb.typeDetailsById(typeObjectId)
}

func (sb *smartBlock) typeDetailsById(typeObjectId string) (*domain.Details, error) {
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

// convertNewObjectLayoutBlocks shapes the blocks of an object that is being created to the layout
// it resolves to: creation templates and imports rely on it for the title and description of
// titled layouts, and to move the name into the first block for note. Creation is a single write
// by the creating device, so unlike a load it cannot be repeated by every member of a space.
// The behaviour is the conversion resolveLayout used to run on every init, kept as is.
func convertNewObjectLayoutBlocks(st *state.State, oldLayout, newLayout domain.Value) {
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
		log.With("objectId", st.RootId()).Infof("convert layout of new object: %s -> %s", oldLayout, newLayout)
		templates := []template.StateTransformer{template.WithNameFromFirstBlock, template.WithTitle}
		if st.Details().GetString(bundle.RelationKeyDescription) != "" {
			templates = append(templates, template.WithDescription)
		}
		template.InitTemplate(st, templates...)
		return
	}
	if !st.Exists(state.TitleBlockID) {
		return
	}
	log.With("objectId", st.RootId()).Infof("convert layout of new object: %s -> %s", oldLayout, newLayout)
	template.InitTemplate(st,
		template.WithNameToFirstBlock,
		template.WithNoTitle,
		template.WithNoDescription,
	)
}
