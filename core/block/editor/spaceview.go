package editor

import (
	"errors"
	"fmt"
	"time"

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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var ErrIncorrectSpaceInfo = errors.New("space info is incorrect")

type spaceService interface {
	OnViewUpdated(info spaceinfo.SpacePersistentInfo)
	OnWorkspaceChanged(spaceId string, details *types.Struct)
}

// SpaceView is a wrapper around smartblock.SmartBlock that indicates the current space state
type SpaceView struct {
	smartblock.SmartBlock
	spaceService      spaceService
	fileObjectService fileobject.Service
}

// newSpaceView creates a new SpaceView with given deps
func (f *ObjectFactory) newSpaceView(sb smartblock.SmartBlock) *SpaceView {
	return &SpaceView{
		SmartBlock:        sb,
		spaceService:      f.spaceService,
		fileObjectService: f.fileObjectService,
	}
}

// Init initializes SpaceView
func (s *SpaceView) Init(ctx *smartblock.InitContext) (err error) {
	if err = s.SmartBlock.Init(ctx); err != nil {
		return
	}
	spaceID, err := s.targetSpaceID()
	if err != nil {
		return
	}

	s.DisableLayouts()
	info := s.getSpaceInfo(ctx.State)
	newPersistentInfo := spaceinfo.SpacePersistentInfo{SpaceID: spaceID, AccountStatus: info.AccountStatus}
	s.setSpacePersistentInfo(ctx.State, newPersistentInfo)
	s.setSpaceLocalInfo(ctx.State, spaceinfo.SpaceLocalInfo{
		SpaceID:      spaceID,
		LocalStatus:  spaceinfo.LocalStatusUnknown,
		RemoteStatus: spaceinfo.RemoteStatusUnknown,
	})
	s.spaceService.OnViewUpdated(newPersistentInfo)
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
		template.WithRelations([]domain.RelationKey{
			bundle.RelationKeySpaceLocalStatus,
			bundle.RelationKeySpaceRemoteStatus,
			bundle.RelationKeyTargetSpaceId,
		}),
	)
}

func (s *SpaceView) TryClose(objectTTL time.Duration) (res bool, err error) {
	return false, nil
}

func (s *SpaceView) SetSpaceLocalInfo(info spaceinfo.SpaceLocalInfo) (err error) {
	st := s.NewState()
	s.setSpaceLocalInfo(st, info)
	return s.Apply(st)
}

func (s *SpaceView) SetAccessType(acc spaceinfo.AccessType) (err error) {
	st := s.NewState()
	prev := spaceinfo.AccessType(pbtypes.GetInt64(st.LocalDetails(), bundle.RelationKeySpaceAccessType.String()))
	// Can't change access level for personal space
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

func (s *SpaceView) SetInviteFileInfo(fileCid string, fileKey string) (err error) {
	st := s.NewState()
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileCid, pbtypes.String(fileCid))
	st.SetDetailAndBundledRelation(bundle.RelationKeySpaceInviteFileKey, pbtypes.String(fileKey))
	return s.Apply(st)
}

func (s *SpaceView) afterApply(info smartblock.ApplyInfo) (err error) {
	s.spaceService.OnViewUpdated(s.getSpaceInfo(info.State))
	return nil
}

func (s *SpaceView) setSpaceLocalInfo(st *state.State, info spaceinfo.SpaceLocalInfo) {
	st.SetLocalDetail(bundle.RelationKeyTargetSpaceId.String(), pbtypes.String(info.SpaceID))
	st.SetLocalDetail(bundle.RelationKeySpaceLocalStatus.String(), pbtypes.Int64(int64(info.LocalStatus)))
	st.SetLocalDetail(bundle.RelationKeySpaceRemoteStatus.String(), pbtypes.Int64(int64(info.RemoteStatus)))
}

func (s *SpaceView) setSpacePersistentInfo(st *state.State, info spaceinfo.SpacePersistentInfo) {
	st.SetLocalDetail(bundle.RelationKeyTargetSpaceId.String(), pbtypes.String(info.SpaceID))
	st.SetDetail(bundle.RelationKeySpaceAccountStatus.String(), pbtypes.Int64(int64(info.AccountStatus)))
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

func (s *SpaceView) getSpaceInfo(st *state.State) (info spaceinfo.SpacePersistentInfo) {
	details := st.CombinedDetails()
	return spaceinfo.SpacePersistentInfo{
		SpaceID:       pbtypes.GetString(details, bundle.RelationKeyTargetSpaceId.String()),
		AccountStatus: spaceinfo.AccountStatus(pbtypes.GetInt64(details, bundle.RelationKeySpaceAccountStatus.String())),
	}
}

var workspaceKeysToCopy = []string{
	bundle.RelationKeyName.String(),
	bundle.RelationKeyIconImage.String(),
	bundle.RelationKeyIconOption.String(),
	bundle.RelationKeySpaceDashboardId.String(),
	bundle.RelationKeyCreator.String(),
	bundle.RelationKeyCreatedDate.String(),
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
