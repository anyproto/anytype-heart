package test

import (
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type MockObject struct{}

func (m MockObject) Init(ctx *smartblock.InitContext) (err error) {
	return nil
}

// nolint: revive
func (m MockObject) Id() string {
	return "id"
}

func (m MockObject) Type() model.SmartBlockType {
	return model.SmartBlockType_Page
}

func (m MockObject) Show(context *session.Context) (obj *model.ObjectView, err error) {
	return nil, nil
}

func (m MockObject) SetEventFunc(f func(e *pb.Event)) {}

func (m MockObject) Apply(s *state.State, flags ...smartblock.ApplyFlag) error {
	return nil
}

func (m MockObject) History() undo.History {
	return nil
}

func (m MockObject) SetDetails(_ *session.Context, _ []*pb.RpcObjectSetDetailsDetail, _ bool) (err error) {
	return nil
}

func (m MockObject) Relations(s *state.State) relationutils.Relations {
	return nil
}

func (m MockObject) HasRelation(s *state.State, relationKey string) bool {
	return true
}

func (m MockObject) AddRelationLinks(ctx *session.Context, relationIds ...string) (err error) {
	return nil
}

func (m MockObject) RemoveExtraRelations(ctx *session.Context, relationKeys []string) (err error) {
	return nil
}

func (m MockObject) TemplateCreateFromObjectState() (*state.State, error) {
	return nil, nil
}

func (m MockObject) SetObjectTypes(ctx *session.Context, objectTypes []string) (err error) {
	return nil
}

func (m MockObject) SetAlign(ctx *session.Context, align model.BlockAlign, ids ...string) error {
	return nil
}

func (m MockObject) SetVerticalAlign(ctx *session.Context, align model.BlockVerticalAlign, ids ...string) error {
	return nil
}

func (m MockObject) SetLayout(ctx *session.Context, layout model.ObjectTypeLayout) error {
	return nil
}

func (m MockObject) SetIsDeleted() {}

func (m MockObject) IsDeleted() bool {
	return false
}

func (m MockObject) IsLocked() bool {
	return false
}

func (m MockObject) SendEvent(msgs []*pb.EventMessage) {}

func (m MockObject) ResetToVersion(s *state.State) (err error) {
	return nil
}

func (m MockObject) DisableLayouts() {}

func (m MockObject) EnabledRelationAsDependentObjects() {}

func (m MockObject) AddHook(f smartblock.HookCallback, events ...smartblock.Hook) {}

func (m MockObject) CheckSubscriptions() (changed bool) {
	return true
}

func (m MockObject) GetDocInfo() (doc.DocInfo, error) {
	return doc.DocInfo{}, nil
}

func (m MockObject) Restrictions() restriction.Restrictions {
	return restriction.Restrictions{}
}

func (m MockObject) SetRestrictions(r restriction.Restrictions) {}

func (m MockObject) ObjectClose() {}

func (m MockObject) FileRelationKeys(s *state.State) []string {
	return nil
}

func (m MockObject) Inner() smartblock.SmartBlock {
	return nil
}

func (m MockObject) Close() (err error) {
	return nil
}

func (m MockObject) Locked() bool {
	return true
}

// nolint: revive
func (m MockObject) RootId() string {
	return ""
}

func (m MockObject) NewState() *state.State {
	return nil
}

func (m MockObject) NewStateCtx(ctx *session.Context) *state.State {
	return nil
}

func (m MockObject) Blocks() []*model.Block {
	return nil
}

func (m MockObject) Pick(id string) (b simple.Block) {
	return nil
}

func (m MockObject) Details() *types.Struct {
	return nil
}

func (m MockObject) CombinedDetails() *types.Struct {
	return nil
}

func (m MockObject) LocalDetails() *types.Struct {
	return nil
}

func (m MockObject) OldExtraRelations() []*model.Relation {
	return nil
}

func (m MockObject) GetRelationLinks() pbtypes.RelationLinks {
	return nil
}

func (m MockObject) ObjectTypes() []string {
	return nil
}

func (m MockObject) ObjectType() string {
	return ""
}

func (m MockObject) Iterate(f func(b simple.Block) (isContinue bool)) (err error) {
	return nil
}

func (m MockObject) Snippet() (snippet string) {
	return ""
}

func (m MockObject) GetAndUnsetFileKeys() []pb.ChangeFileKeys {
	return nil
}

func (m MockObject) BlocksInit(ds simple.DetailsService) {}

func (m MockObject) SearchText() string {
	return ""
}

func (m MockObject) GetFirstTextBlock() (*model.BlockContentOfText, error) {
	return nil, nil
}

func (m MockObject) Lock() {}

func (m MockObject) Unlock() {}
