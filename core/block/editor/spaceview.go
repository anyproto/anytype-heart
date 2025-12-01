package editor

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/order"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

var spaceViewLog = logging.Logger("core.block.editor.spaceview")

var ErrIncorrectSpaceInfo = errors.New("space info is incorrect")

// required relations for spaceview beside the bundle.RequiredInternalRelations
var spaceViewRequiredRelations = []domain.RelationKey{
	bundle.RelationKeySpaceLocalStatus,
	bundle.RelationKeySpaceRemoteStatus,
	bundle.RelationKeyTargetSpaceId,
	bundle.RelationKeyIsAclShared,
	bundle.RelationKeySharedSpacesLimit,
	bundle.RelationKeySpaceAccountStatus,
	bundle.RelationKeySpaceShareableStatus,
	bundle.RelationKeySpaceAccessType,
	bundle.RelationKeySpaceUxType,
	bundle.RelationKeyLatestAclHeadId,
	bundle.RelationKeyChatId,
	bundle.RelationKeyReadersLimit,
	bundle.RelationKeyWritersLimit,
}

type spaceService interface {
	OnWorkspaceChanged(spaceId string, details *domain.Details)
	PersonalSpaceId() string
}

// SpaceView is a wrapper around smartblock.SmartBlock that indicates the current space state
type SpaceView struct {
	smartblock.SmartBlock
	order.OrderSettable
	spaceService      spaceService
	fileObjectService fileobject.Service
	log               *logging.Sugared
}

// newSpaceView creates a new SpaceView with given deps
func (f *ObjectFactory) newSpaceView(sb smartblock.SmartBlock) *SpaceView {
	return &SpaceView{
		SmartBlock:        sb,
		OrderSettable:     order.NewOrderSettable(sb, bundle.RelationKeySpaceOrder),
		spaceService:      f.spaceService,
		log:               spaceViewLog,
		fileObjectService: f.fileObjectService,
	}
}

// Init initializes SpaceView
func (s *SpaceView) Init(ctx *smartblock.InitContext) (err error) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, spaceViewRequiredRelations...)
	if err = s.SmartBlock.Init(ctx); err != nil {
		return
	}
	spaceId, err := s.targetSpaceID()
	if err != nil {
		return
	}
	s.log = s.log.With("spaceId", spaceId)

	info := spaceinfo.NewSpacePersistentInfoFromState(ctx.State)
	newInfo := spaceinfo.NewSpacePersistentInfo(spaceId)
	newInfo.SetAccountStatus(info.GetAccountStatus()).
		SetAclHeadId(info.GetAclHeadId()).
		SetEncodedKey(info.EncodedKey)
	s.setSpacePersistentInfo(ctx.State, newInfo)
	localInfo := spaceinfo.NewSpaceLocalInfo(spaceId)
	localInfo.SetLocalStatus(spaceinfo.LocalStatusUnknown).
		SetRemoteStatus(spaceinfo.RemoteStatusUnknown).
		UpdateDetails(ctx.State).
		Log(log)
	s.AddHook(s.afterApply, smartblock.HookAfterApply)
	return
}

func (s *SpaceView) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 2,
		Proc:    s.initTemplate,
	}
}

func (s *SpaceView) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{
		{
			Version: 2,
			Proc:    s.initTemplate,
		},
	})
}

func (s *SpaceView) initTemplate(st *state.State) {
	template.InitTemplate(st,
		template.WithObjectTypes([]domain.TypeKey{bundle.TypeKeySpaceView}),
		template.WithLayout(model.ObjectType_spaceView),
	)
}

func (s *SpaceView) TryClose(objectTTL time.Duration) (res bool, err error) {
	return false, nil
}

func (s *SpaceView) SetSpaceLocalInfo(info spaceinfo.SpaceLocalInfo) (err error) {
	st := s.NewState()
	info.UpdateDetails(st).Log(log)
	s.updateAccessType(st)
	return s.Apply(st)
}

func (s *SpaceView) SetOwner(ownerId string, createdDate int64) (err error) {
	st := s.NewState()
	if createdDate != 0 {
		st.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, domain.Int64(createdDate))
	}
	st.SetDetailAndBundledRelation(bundle.RelationKeyCreator, domain.String(ownerId))
	return s.Apply(st)
}

func (s *SpaceView) SetMyParticipantStatus(status model.ParticipantStatus) (err error) {
	st := s.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeyMyParticipantStatus, domain.Int64(int64(status)))
	return s.Apply(st)
}

func (s *SpaceView) SetAclInfo(isAclEmpty bool, pushKey crypto.PrivKey, pushEncKey crypto.SymKey, joinedDate int64) error {
	st := s.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeyIsAclShared, domain.Bool(!isAclEmpty))

	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceJoinDate, domain.Int64(joinedDate))

	if pushKey != nil {
		pushKeyBinary, err := pushKey.Marshall()
		if err != nil {
			return err
		}
		pushKeyString := base64.StdEncoding.EncodeToString(pushKeyBinary)
		st.SetDetailAndBundledRelation(bundle.RelationKeySpacePushNotificationKey, domain.String(pushKeyString))
	}

	if pushEncKey != nil {
		pushEncBinary, err := pushEncKey.Raw()
		if err != nil {
			return err
		}
		pushEncString := base64.StdEncoding.EncodeToString(pushEncBinary)
		st.SetDetailAndBundledRelation(bundle.RelationKeySpacePushNotificationEncryptionKey, domain.String(pushEncString))
	}

	s.updateAccessType(st)
	return s.Apply(st)
}

var (
	pushNotificationIdsRelByMode = map[pb.RpcPushNotificationMode]domain.RelationKey{
		pb.RpcPushNotification_All:      bundle.RelationKeySpacePushNotificationForceAllIds,
		pb.RpcPushNotification_Mentions: bundle.RelationKeySpacePushNotificationForceMentionIds,
		pb.RpcPushNotification_Nothing:  bundle.RelationKeySpacePushNotificationForceMuteIds,
	}
)

func (s *SpaceView) SetPushNotificationForceModeIds(ctx session.Context, chatIds []string, mode pb.RpcPushNotificationMode) (err error) {
	if _, ok := pushNotificationIdsRelByMode[mode]; !ok {
		return fmt.Errorf("unknown push notification mode: %v", mode)
	}

	st := s.NewStateCtx(ctx)
	details := st.Details()
	for keyMode, key := range pushNotificationIdsRelByMode {
		if keyMode == mode {
			existingIds := details.GetStringList(key)
			for _, id := range chatIds {
				if !slices.Contains(existingIds, id) {
					existingIds = append(existingIds, id)
				}
			}
			st.SetDetail(key, domain.StringList(existingIds))
		} else {
			existingIds := details.GetStringList(key)
			filteredIds := existingIds[:0]
			for _, id := range existingIds {
				if !slices.Contains(chatIds, id) {
					filteredIds = append(filteredIds, id)
				}
			}
			st.SetDetail(key, domain.StringList(filteredIds))
		}
	}
	return s.Apply(st)
}

func (s *SpaceView) ResetPushNotificationIds(ctx session.Context, chatIds []string) (err error) {
	st := s.NewStateCtx(ctx)
	details := st.Details()

	for _, key := range pushNotificationIdsRelByMode {
		existingIds := details.GetStringList(key)
		filteredIds := existingIds[:0]
		for _, id := range existingIds {
			if !slices.Contains(chatIds, id) {
				filteredIds = append(filteredIds, id)
			}
		}
		st.SetDetail(key, domain.StringList(filteredIds))
	}

	return s.Apply(st)
}

func (s *SpaceView) updateAccessType(st *state.State) {
	accessType := spaceinfo.AccessType(st.LocalDetails().GetInt64(bundle.RelationKeySpaceAccessType))
	if accessType == spaceinfo.AccessTypePersonal {
		return
	}
	isShared := st.LocalDetails().GetBool(bundle.RelationKeyIsAclShared)
	shareable := spaceinfo.ShareableStatus(st.LocalDetails().GetInt64(bundle.RelationKeySpaceShareableStatus))
	if isShared || shareable == spaceinfo.ShareableStatusShareable {
		stateSetAccessType(st, spaceinfo.AccessTypeShared)
	} else {
		stateSetAccessType(st, spaceinfo.AccessTypePrivate)
	}
}

func (s *SpaceView) SetAccessType(acc spaceinfo.AccessType) (err error) {
	st := s.NewState()
	prev := spaceinfo.AccessType(st.LocalDetails().GetInt64(bundle.RelationKeySpaceAccessType))
	if prev == spaceinfo.AccessTypePersonal {
		return nil
	}
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceAccessType, domain.Int64(acc))
	return s.Apply(st)
}

func (s *SpaceView) SetSpacePersistentInfo(info spaceinfo.SpacePersistentInfo) (err error) {
	st := s.NewState()
	s.setSpacePersistentInfo(st, info)
	return s.Apply(st)
}

func (s *SpaceView) SetSharedSpacesLimit(limit int) (err error) {
	st := s.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeySharedSpacesLimit, domain.Int64(limit))
	return s.Apply(st)
}

func (s *SpaceView) SetPushNotificationMode(ctx session.Context, mode pb.RpcPushNotificationMode) (err error) {
	st := s.NewStateCtx(ctx)
	st.SetDetailAndBundledRelation(bundle.RelationKeySpacePushNotificationMode, domain.Int64(mode))
	return s.Apply(st)
}

func (s *SpaceView) GetSharedSpacesLimit() (limit int) {
	return int(s.CombinedDetails().GetInt64(bundle.RelationKeySharedSpacesLimit))
}

func (s *SpaceView) afterApply(info smartblock.ApplyInfo) (err error) {
	return nil
}

func (s *SpaceView) GetLocalInfo() spaceinfo.SpaceLocalInfo {
	return spaceinfo.NewSpaceLocalInfoFromState(s)
}

func (s *SpaceView) GetPersistentInfo() spaceinfo.SpacePersistentInfo {
	return spaceinfo.NewSpacePersistentInfoFromState(s)
}

func (s *SpaceView) setSpacePersistentInfo(st *state.State, info spaceinfo.SpacePersistentInfo) {
	info.UpdateDetails(st)
	info.Log(s.log)
}

// targetSpaceID returns space id from the root of space object's tree
func (s *SpaceView) targetSpaceID() (id string, err error) {
	changeInfo := s.Tree().ChangeInfo()
	if changeInfo == nil {
		return "", ErrIncorrectSpaceInfo
	}
	changePayload := &model.ObjectChangePayload{}
	err = proto.Unmarshal(changeInfo.ChangePayload, changePayload)
	if err != nil {
		return "", ErrIncorrectSpaceInfo
	}
	if changePayload.Key == "" {
		return "", fmt.Errorf("space key is empty")
	}
	return changePayload.Key, nil
}

var workspaceKeysToCopy = []domain.RelationKey{
	bundle.RelationKeyName,
	bundle.RelationKeyIconImage,
	bundle.RelationKeyIconOption,
	bundle.RelationKeySpaceDashboardId,
	bundle.RelationKeySpaceUxType,
	bundle.RelationKeyCreatedDate,
	bundle.RelationKeyChatId,
	bundle.RelationKeyDescription,
	bundle.RelationKeyOneToOneIdentity,
	bundle.RelationKeyAnalyticsSpaceId,
}

func (s *SpaceView) GetSpaceDescription() spaceinfo.SpaceDescription {
	details := s.CombinedDetails()
	return spaceinfo.NewSpaceDescriptionFromDetails(details)
}

func (s *SpaceView) SetSpaceData(details *domain.Details) error {
	st := s.NewState()
	var changed bool
	for k, v := range details.Iterate() {
		if slices.Contains(workspaceKeysToCopy, k) {
			// Special case for migration to Files as Objects to handle following situation:
			// - We have an icon in Workspace that was created in pre-Files as Objects version
			// - We migrate it, change old id to new id
			// - Now we need to push details to SpaceView. But if we push NEW id, then old clients will not be able to display image
			// - So we need to push old id
			if k == bundle.RelationKeyIconImage {
				fileId, err := s.fileObjectService.GetFileIdFromObject(v.String())
				if err == nil {
					v = domain.String(fileId.FileId.String())
				}
			}
			if k == bundle.RelationKeyCreatedDate && s.GetLocalInfo().SpaceId != s.spaceService.PersonalSpaceId() {
				continue
			}
			changed = true
			st.SetDetailAndBundledRelation(k, v)
		}
	}

	if changed {
		if st.ParentState().ParentState() == nil {
			// in case prev change was the first one
			createdDate := details.GetInt64(bundle.RelationKeyCreatedDate)
			if createdDate > 0 {
				// we use this state field to save the original created date, otherwise we use the one from the underlying objectTree
				st.SetOriginalCreatedTimestamp(createdDate)
			}
		}

		return s.Apply(st, smartblock.NoRestrictions, smartblock.NoEvent, smartblock.NoHistory)
	}
	return nil
}

func (s *SpaceView) UpdateLastOpenedDate() error {
	st := s.NewState()
	st.SetLocalDetail(bundle.RelationKeyLastOpenedDate, domain.Int64(time.Now().Unix()))
	return s.Apply(st, smartblock.NoHistory, smartblock.NoEvent, smartblock.KeepInternalFlags)
}

func stateSetAccessType(st *state.State, accessType spaceinfo.AccessType) {
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceAccessType, domain.Int64(accessType))
}
