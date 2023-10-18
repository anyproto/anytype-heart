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
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var ErrIncorrectSpaceInfo = errors.New("space info is incorrect")

type spaceService interface {
	OnViewCreated(info spaceinfo.SpaceInfo)
	OnWorkspaceChanged(spaceId string, details *types.Struct)
}

// SpaceView is a wrapper around smartblock.SmartBlock that indicates the current space state
type SpaceView struct {
	smartblock.SmartBlock
	spaceService spaceService
}

// newSpaceView creates a new SpaceView with given deps
func newSpaceView(sb smartblock.SmartBlock, spaceService spaceService) *SpaceView {
	return &SpaceView{
		SmartBlock:   sb,
		spaceService: spaceService,
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
	newInfo := spaceinfo.SpaceInfo{SpaceID: spaceID, AccountStatus: info.AccountStatus}
	s.setSpaceInfo(ctx.State, newInfo)
	s.spaceService.OnViewCreated(newInfo)
	return
}

func (s *SpaceView) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 1,
		Proc: func(s *state.State) {
			template.InitTemplate(s,
				template.WithObjectTypesAndLayout([]domain.TypeKey{bundle.TypeKeySpaceView}, model.ObjectType_spaceView),
				template.WithRelations([]domain.RelationKey{
					bundle.RelationKeySpaceLocalStatus,
					bundle.RelationKeySpaceRemoteStatus,
					bundle.RelationKeyTargetSpaceId,
				}),
			)
		},
	}
}

func (s *SpaceView) StateMigrations() migration.Migrations {
	return migration.MakeMigrations(nil)
}

func (s *SpaceView) TryClose(objectTTL time.Duration) (res bool, err error) {
	return false, nil
}

func (s *SpaceView) SetSpaceInfo(info spaceinfo.SpaceInfo) (err error) {
	st := s.NewState()
	s.setSpaceInfo(st, info)
	return s.Apply(st)
}

func (s *SpaceView) setSpaceInfo(st *state.State, info spaceinfo.SpaceInfo) {
	st.SetLocalDetail(bundle.RelationKeyTargetSpaceId.String(), pbtypes.String(info.SpaceID))
	st.SetLocalDetail(bundle.RelationKeySpaceLocalStatus.String(), pbtypes.Int64(int64(info.LocalStatus)))
	st.SetLocalDetail(bundle.RelationKeySpaceRemoteStatus.String(), pbtypes.Int64(int64(info.RemoteStatus)))
	st.SetDetail(bundle.RelationKeySpaceAccountStatus.String(), pbtypes.Int64(int64(info.AccountStatus)))
	return
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

func (s *SpaceView) getSpaceInfo(st *state.State) (info spaceinfo.SpaceInfo) {
	details := st.CombinedDetails()
	return spaceinfo.SpaceInfo{
		SpaceID:       pbtypes.GetString(details, bundle.RelationKeyTargetSpaceId.String()),
		LocalStatus:   spaceinfo.LocalStatus(pbtypes.GetInt64(details, bundle.RelationKeySpaceLocalStatus.String())),
		RemoteStatus:  spaceinfo.RemoteStatus(pbtypes.GetInt64(details, bundle.RelationKeySpaceRemoteStatus.String())),
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
			changed = true
			st.SetDetailAndBundledRelation(domain.RelationKey(k), v)
		}
	}

	if changed {
		return s.Apply(st, smartblock.NoRestrictions, smartblock.NoEvent, smartblock.NoHistory)
	}
	return nil
}

func (s *SpaceView) UpdateLastOpenedDate() error {
	st := s.NewState()
	st.SetLocalDetail(bundle.RelationKeyLastOpenedDate.String(), pbtypes.Int64(time.Now().Unix()))
	return s.Apply(st, smartblock.NoHistory, smartblock.NoEvent, smartblock.SkipIfNoChanges, smartblock.KeepInternalFlags)
}
