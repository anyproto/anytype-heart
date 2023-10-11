package editor

import (
	"errors"
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
	OnViewCreated(spaceId string)
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
	s.spaceService.OnViewCreated(spaceID)
	return s.setSpaceInfo(ctx.State, spaceinfo.SpaceInfo{})
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
	if err = s.setSpaceInfo(st, info); err != nil {
		return
	}
	return s.Apply(st)
}

func (s *SpaceView) setSpaceInfo(st *state.State, info spaceinfo.SpaceInfo) (err error) {
	st.SetLocalDetail(bundle.RelationKeySpaceLocalStatus.String(), pbtypes.Int64(int64(info.LocalStatus)))
	st.SetLocalDetail(bundle.RelationKeySpaceRemoteStatus.String(), pbtypes.Int64(int64(info.RemoteStatus)))
	return
}

// targetSpaceID returns space id from the root of space object's tree
func (s *SpaceView) targetSpaceID() (id string, err error) {
	changeInfo := s.Tree().ChangeInfo()
	if changeInfo == nil {
		return "", ErrIncorrectSpaceInfo
	}
	var (
		changePayload = &model.ObjectChangePayload{}
		spaceHeader   = &model.SpaceObjectHeader{}
	)
	err = proto.Unmarshal(changeInfo.ChangePayload, changePayload)
	if err != nil {
		return "", ErrIncorrectSpaceInfo
	}
	err = proto.Unmarshal(changePayload.Data, spaceHeader)
	if err != nil {
		return "", ErrIncorrectSpaceInfo
	}
	return spaceHeader.SpaceID, nil
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
