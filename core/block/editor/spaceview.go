package editor

import (
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/lexid"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
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
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var spaceViewLog = logging.Logger("core.block.editor.spaceview")

var ErrIncorrectSpaceInfo = errors.New("space info is incorrect")

var lx = lexid.Must(lexid.CharsAll, 4, 10)

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
}

type spaceService interface {
	OnViewUpdated(info spaceinfo.SpacePersistentInfo)
	OnWorkspaceChanged(spaceId string, details *types.Struct)
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

	s.DisableLayouts()
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
	fileCid = pbtypes.GetString(details, bundle.RelationKeySpaceInviteFileCid.String())
	fileKey = pbtypes.GetString(details, bundle.RelationKeySpaceInviteFileKey.String())
	return
}

func (s *SpaceView) RemoveExistingInviteInfo() (fileCid string, err error) {
	details := s.Details()
	fileCid = pbtypes.GetString(details, bundle.RelationKeySpaceInviteFileCid.String())
	newState := s.NewState()
	newState.RemoveDetail(bundle.RelationKeySpaceInviteFileCid.String(), bundle.RelationKeySpaceInviteFileKey.String())
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
		st.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Int64(createdDate))
	}
	st.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(ownerId))
	return s.Apply(st)
}

func (s *SpaceView) SetAclIsEmpty(isEmpty bool) (err error) {
	st := s.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeyIsAclShared, pbtypes.Bool(!isEmpty))
	s.updateAccessType(st)
	return s.Apply(st)
}

func (s *SpaceView) updateAccessType(st *state.State) {
	accessType := spaceinfo.AccessType(pbtypes.GetInt64(st.LocalDetails(), bundle.RelationKeySpaceAccessType.String()))
	if accessType == spaceinfo.AccessTypePersonal {
		return
	}
	isShared := pbtypes.GetBool(st.LocalDetails(), bundle.RelationKeyIsAclShared.String())
	shareable := spaceinfo.ShareableStatus(pbtypes.GetInt64(st.LocalDetails(), bundle.RelationKeySpaceShareableStatus.String()))
	if isShared || shareable == spaceinfo.ShareableStatusShareable {
		stateSetAccessType(st, spaceinfo.AccessTypeShared)
	} else {
		stateSetAccessType(st, spaceinfo.AccessTypePrivate)
	}
}

func (s *SpaceView) SetAccessType(acc spaceinfo.AccessType) (err error) {
	st := s.NewState()
	prev := spaceinfo.AccessType(pbtypes.GetInt64(st.LocalDetails(), bundle.RelationKeySpaceAccessType.String()))
	if prev == spaceinfo.AccessTypePersonal {
		return nil
	}
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceAccessType, pbtypes.Int64(int64(acc)))
	return s.Apply(st)
}

func (s *SpaceView) SetSpacePersistentInfo(info spaceinfo.SpacePersistentInfo) (err error) {
	st := s.NewState()
	s.setSpacePersistentInfo(st, info)
	return s.Apply(st)
}

func (s *SpaceView) SetSharedSpacesLimit(limit int) (err error) {
	st := s.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeySharedSpacesLimit, pbtypes.Int64(int64(limit)))
	return s.Apply(st)
}

func (s *SpaceView) GetSharedSpacesLimit() (limit int) {
	return int(pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeySharedSpacesLimit.String()))
}

func (s *SpaceView) SetInviteFileInfo(fileCid string, fileKey string) (err error) {
	st := s.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileCid, pbtypes.String(fileCid))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileKey, pbtypes.String(fileKey))
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
	spaceInfo := spaceinfo.NewSpacePersistentInfo(pbtypes.GetString(details, bundle.RelationKeyTargetSpaceId.String()))
	spaceInfo.SetAccountStatus(spaceinfo.AccountStatus(pbtypes.GetInt64(details, bundle.RelationKeySpaceAccountStatus.String()))).
		SetAclHeadId(pbtypes.GetString(details, bundle.RelationKeyLatestAclHeadId.String()))
	return spaceInfo
}

var workspaceKeysToCopy = []string{
	bundle.RelationKeyName.String(),
	bundle.RelationKeyIconImage.String(),
	bundle.RelationKeyIconOption.String(),
	bundle.RelationKeySpaceDashboardId.String(),
	bundle.RelationKeyCreatedDate.String(),
}

func (s *SpaceView) GetSpaceDescription() (data spaceinfo.SpaceDescription) {
	details := s.CombinedDetails()
	data.Name = pbtypes.GetString(details, bundle.RelationKeyName.String())
	data.IconImage = pbtypes.GetString(details, bundle.RelationKeyIconImage.String())
	return
}

func (s *SpaceView) SetSpaceData(details *types.Struct) error {
	st := s.NewState()
	var changed bool
	for k, v := range details.Fields {
		if slices.Contains(workspaceKeysToCopy, k) {
			// Special case for migration to Files as Objects to handle following situation:
			// - We have an icon in Workspace that was created in pre-Files as Objects version
			// - We migrate it, change old id to new id
			// - Now we need to push details to SpaceView. But if we push NEW id, then old clients will not be able to display image
			// - So we need to push old id
			if k == bundle.RelationKeyIconImage.String() {
				fileId, err := s.fileObjectService.GetFileIdFromObject(v.GetStringValue())
				if err == nil {
					switch v.Kind.(type) {
					case *types.Value_StringValue:
						v = pbtypes.String(fileId.FileId.String())
					case *types.Value_ListValue:
						v = pbtypes.StringList([]string{fileId.FileId.String()})
					}
				}
			}
			if k == bundle.RelationKeyCreatedDate.String() && s.GetLocalInfo().SpaceId != s.spaceService.PersonalSpaceId() {
				continue
			}
			changed = true
			st.SetDetailAndBundledRelation(domain.RelationKey(k), v)
		}
	}

	if changed {
		if st.ParentState().ParentState() == nil {
			// in case prev change was the first one
			createdDate := pbtypes.GetInt64(details, bundle.RelationKeyCreatedDate.String())
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
	st.SetLocalDetail(bundle.RelationKeyLastOpenedDate.String(), pbtypes.Int64(time.Now().Unix()))
	return s.Apply(st, smartblock.NoHistory, smartblock.NoEvent, smartblock.SkipIfNoChanges, smartblock.KeepInternalFlags)
}

func (s *SpaceView) SetAfterGivenView(viewOrderId string) (string, error) {
	st := s.NewState()
	spaceOrderId := pbtypes.GetString(st.Details(), bundle.RelationKeySpaceOrder.String())
	if spaceOrderId == "" || viewOrderId > spaceOrderId {
		spaceOrderId = lx.Next(viewOrderId)
		st.SetDetail(bundle.RelationKeySpaceOrder.String(), pbtypes.String(spaceOrderId))
		return spaceOrderId, s.Apply(st)
	}
	return spaceOrderId, nil
}

func (s *SpaceView) SetBetweenViews(prevViewOrderId, afterViewOrderId string) error {
	st := s.NewState()
	before, err := lx.NextBefore(prevViewOrderId, afterViewOrderId)
	if err != nil {
		return fmt.Errorf("failed to get before lexid, %w", err)
	}
	st.SetDetail(bundle.RelationKeySpaceOrder.String(), pbtypes.String(before))
	return s.Apply(st)
}

func stateSetAccessType(st *state.State, accessType spaceinfo.AccessType) {
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceAccessType, pbtypes.Int64(int64(accessType)))
}
