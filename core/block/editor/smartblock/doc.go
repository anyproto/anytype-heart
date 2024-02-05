package smartblock

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (sb *smartBlock) currentState() *state.State {
	if sb.pendingState != nil {
		return sb.pendingState
	}
	return sb.state
}

var _ state.Doc = (*smartBlock)(nil)

/*
type Doc interface {
	RootId() string
	NewState() *State
	NewStateCtx(ctx session.Context) *State
	Blocks() []*model.Block
	Pick(id string) (b simple.Block)
	Details() *types.Struct
	CombinedDetails() *types.Struct
	LocalDetails() *types.Struct

	GetRelationLinks() pbtypes.RelationLinks

	ObjectTypeKeys() []domain.TypeKey
	ObjectTypeKey() domain.TypeKey
	Layout() (model.ObjectTypeLayout, bool)

	Iterate(f func(b simple.Block) (isContinue bool)) (err error)
	Snippet() (snippet string)
	UniqueKeyInternal() string

	GetAndUnsetFileKeys() []pb.ChangeFileKeys
	BlocksInit(ds simple.DetailsService)
	SearchText() string
	ChangeId() string // last pushed change id
}
*/

func (sb *smartBlock) RootId() string {
	return sb.currentState().RootId()
}

func (sb *smartBlock) NewState() *state.State {
	if sb.pendingState != nil {
		return sb.pendingState
	}
	s := sb.state.NewState().SetNoObjectType(sb.Type() == smartblock.SmartBlockTypeArchive)
	sb.execHooks(HookOnNewState, ApplyInfo{State: s})
	return s
}

func (sb *smartBlock) NewStateCtx(ctx session.Context) *state.State {
	if sb.pendingState != nil {
		sb.pendingState.SetContext(ctx)
		return sb.pendingState
	}
	s := sb.state.NewStateCtx(ctx).SetNoObjectType(sb.Type() == smartblock.SmartBlockTypeArchive)
	sb.execHooks(HookOnNewState, ApplyInfo{State: s})
	return s
}

func (sb *smartBlock) Blocks() []*model.Block {
	return sb.currentState().Blocks()
}

func (sb *smartBlock) Pick(id string) (b simple.Block) {
	return sb.currentState().Pick(id)
}

func (sb *smartBlock) Details() *types.Struct {
	return sb.currentState().Details()
}

func (sb *smartBlock) CombinedDetails() *types.Struct {
	return sb.currentState().CombinedDetails()
}

func (sb *smartBlock) LocalDetails() *types.Struct {
	return sb.currentState().LocalDetails()
}

func (sb *smartBlock) GetRelationLinks() pbtypes.RelationLinks {
	return sb.currentState().GetRelationLinks()
}

func (sb *smartBlock) ObjectTypeKeys() []domain.TypeKey {
	return sb.currentState().ObjectTypeKeys()
}

func (sb *smartBlock) ObjectTypeKey() domain.TypeKey {
	return sb.currentState().ObjectTypeKey()
}

func (sb *smartBlock) Layout() (model.ObjectTypeLayout, bool) {
	return sb.currentState().Layout()
}

func (sb *smartBlock) Iterate(f func(b simple.Block) (isContinue bool)) (err error) {
	return sb.currentState().Iterate(f)
}

func (sb *smartBlock) Snippet() (snippet string) {
	return sb.currentState().Snippet()
}

func (sb *smartBlock) UniqueKeyInternal() string {
	return sb.currentState().UniqueKeyInternal()
}

func (sb *smartBlock) GetAndUnsetFileKeys() (keys []pb.ChangeFileKeys) {
	keys2 := sb.source.GetFileKeysSnapshot()
	for _, key := range keys2 {
		if key == nil {
			continue
		}
		keys = append(keys, pb.ChangeFileKeys{
			Hash: key.Hash,
			Keys: key.Keys,
		})
	}
	return
}

func (sb *smartBlock) BlocksInit(ds simple.DetailsService) {
	sb.currentState().BlocksInit(ds)
}

func (sb *smartBlock) SearchText() string {
	return sb.currentState().SearchText()
}

func (sb *smartBlock) ChangeId() string {
	return sb.currentState().ChangeId()
}
