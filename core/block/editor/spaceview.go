package editor

import (
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/lexid"
	"github.com/gogo/protobuf/proto"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

var spaceViewLog = logging.Logger("core.block.editor.spaceview")

var ErrIncorrectSpaceInfo = errors.New("space info is incorrect")

var lx = lexid.Must(lexid.CharsBase64, 4, 1000)

// required relations for spaceview beside the bundle.RequiredInternalRelations
var spaceViewRequiredRelations = []domain.RelationKey{
	bundle.RelationKeySpaceLocalStatus,
	bundle.RelationKeySpaceRemoteStatus,
	bundle.RelationKeyTargetSpaceId,
	bundle.RelationKeySpaceInviteFileCid,
	bundle.RelationKeySpaceInviteFileKey,
	bundle.RelationKeyIsAclShared,
	bundle.RelationKeySharedSpacesLimit,
	bundle.RelationKeySpaceAccountStatus,
	bundle.RelationKeySpaceShareableStatus,
	bundle.RelationKeySpaceAccessType,
	bundle.RelationKeyLatestAclHeadId,
	bundle.RelationKeyChatId,
	bundle.RelationKeyReadersLimit,
	bundle.RelationKeyWritersLimit,
}

type spaceService interface {
	OnViewUpdated(info spaceinfo.SpacePersistentInfo)
	OnWorkspaceChanged(spaceId string, details *domain.Details)
	PersonalSpaceId() string
}

// SpaceView is a wrapper around smartblock.SmartBlock that indicates the current space state
type SpaceView struct {
	smartblock.SmartBlock
	spaceService      spaceService
	fileObjectService fileobject.Service
	log               *logging.Sugared
}

// newSpaceView creates a new SpaceView with given deps
func (f *ObjectFactory) newSpaceView(sb smartblock.SmartBlock) *SpaceView {
	return &SpaceView{
		SmartBlock:        sb,
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
		SetAclHeadId(info.GetAclHeadId())
	s.setSpacePersistentInfo(ctx.State, newInfo)
	localInfo := spaceinfo.NewSpaceLocalInfo(spaceId)
	localInfo.SetLocalStatus(spaceinfo.LocalStatusUnknown).
		SetRemoteStatus(spaceinfo.RemoteStatusUnknown).
		UpdateDetails(ctx.State).
		Log(log)
	s.spaceService.OnViewUpdated(newInfo)
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
		template.WithObjectTypesAndLayout([]domain.TypeKey{bundle.TypeKeySpaceView}, model.ObjectType_spaceView),
	)
}

func (s *SpaceView) GetExistingInviteInfo() (fileCid string, fileKey string) {
	details := s.CombinedDetails()
	fileCid = details.GetString(bundle.RelationKeySpaceInviteFileCid)
	fileKey = details.GetString(bundle.RelationKeySpaceInviteFileKey)
	return
}

func (s *SpaceView) RemoveExistingInviteInfo() (fileCid string, err error) {
	details := s.Details()
	fileCid = details.GetString(bundle.RelationKeySpaceInviteFileCid)
	newState := s.NewState()
	newState.RemoveDetail(bundle.RelationKeySpaceInviteFileCid, bundle.RelationKeySpaceInviteFileKey)
	return fileCid, s.Apply(newState)
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

func (s *SpaceView) SetAclIsEmpty(isEmpty bool) (err error) {
	st := s.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeyIsAclShared, domain.Bool(!isEmpty))
	s.updateAccessType(st)
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

func (s *SpaceView) GetSharedSpacesLimit() (limit int) {
	return int(s.CombinedDetails().GetInt64(bundle.RelationKeySharedSpacesLimit))
}

func (s *SpaceView) SetInviteFileInfo(fileCid string, fileKey string) (err error) {
	st := s.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileCid, domain.String(fileCid))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileKey, domain.String(fileKey))
	return s.Apply(st)
}

func (s *SpaceView) afterApply(info smartblock.ApplyInfo) (err error) {
	s.spaceService.OnViewUpdated(s.getSpacePersistentInfo(info.State))
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

func (s *SpaceView) getSpacePersistentInfo(st *state.State) (info spaceinfo.SpacePersistentInfo) {
	details := st.CombinedDetails()
	spaceInfo := spaceinfo.NewSpacePersistentInfo(details.GetString(bundle.RelationKeyTargetSpaceId))
	spaceInfo.SetAccountStatus(spaceinfo.AccountStatus(details.GetInt64(bundle.RelationKeySpaceAccountStatus))).
		SetAclHeadId(details.GetString(bundle.RelationKeyLatestAclHeadId))
	return spaceInfo
}

var workspaceKeysToCopy = []domain.RelationKey{
	bundle.RelationKeyName,
	bundle.RelationKeyIconImage,
	bundle.RelationKeyIconOption,
	bundle.RelationKeySpaceDashboardId,
	bundle.RelationKeyCreatedDate,
	bundle.RelationKeyChatId,
}

func (s *SpaceView) GetSpaceDescription() (data spaceinfo.SpaceDescription) {
	details := s.CombinedDetails()
	data.Name = details.GetString(bundle.RelationKeyName)
	data.IconImage = details.GetString(bundle.RelationKeyIconImage)
	return
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
	return s.Apply(st, smartblock.NoHistory, smartblock.NoEvent, smartblock.SkipIfNoChanges, smartblock.KeepInternalFlags)
}

func (s *SpaceView) SetOrder(prevViewOrderId string) (string, error) {
	st := s.NewState()
	spaceOrderId := lx.Next(prevViewOrderId)
	st.SetDetail(bundle.RelationKeySpaceOrder, domain.String(spaceOrderId))
	return spaceOrderId, s.Apply(st)
}

func (s *SpaceView) SetAfterGivenView(viewOrderId string) error {
	st := s.NewState()
	spaceOrderId := st.Details().GetString(bundle.RelationKeySpaceOrder)
	if viewOrderId > spaceOrderId {
		spaceOrderId = lx.Next(viewOrderId)
		st.SetDetail(bundle.RelationKeySpaceOrder, domain.String(spaceOrderId))
		return s.Apply(st)
	}
	return nil
}

func (s *SpaceView) SetBetweenViews(prevViewOrderId, afterViewOrderId string) error {
	st := s.NewState()
	before, err := lx.NextBefore(prevViewOrderId, afterViewOrderId)
	if err != nil {
		return fmt.Errorf("failed to get before lexid, %w", err)
	}
	st.SetDetail(bundle.RelationKeySpaceOrder, domain.String(before))
	return s.Apply(st)
}

func stateSetAccessType(st *state.State, accessType spaceinfo.AccessType) {
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceAccessType, domain.Int64(accessType))
}
