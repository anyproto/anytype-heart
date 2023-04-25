package smarttest

import (
	"sync"

	"github.com/anytypeio/any-sync/app"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
)

func New(id string) *SmartTest {
	return &SmartTest{
		id:   id,
		Doc:  state.NewDoc(id, nil),
		hist: undo.NewHistory(0),
	}
}

type SmartTest struct {
	Results          Results
	anytype          *testMock.MockService
	id               string
	hist             undo.History
	meta             *core.SmartBlockMeta
	TestRestrictions restriction.Restrictions
	App              *app.App
	sync.Mutex
	state.Doc
	isDeleted bool
	os        *testMock.MockObjectStore
}

func (st *SmartTest) EnabledRelationAsDependentObjects() {
	return
}

func (st *SmartTest) IsLocked() bool {
	return false
}

func (st *SmartTest) RelationService() relation.Service {
	return st.App.Component(relation.CName).(relation.Service)
}

func (st *SmartTest) Locked() bool {
	return false
}

func (st *SmartTest) DocService() doc.Service {
	return nil
}

func (st *SmartTest) ObjectStore() objectstore.ObjectStore {
	return st.os
}

func (st *SmartTest) SetIsDeleted() {
	st.isDeleted = true
}

func (st *SmartTest) IsDeleted() bool {
	return st.isDeleted
}

func (st *SmartTest) GetFirstTextBlock() (*model.BlockContentOfText, error) {
	return nil, nil
}

func (st *SmartTest) SetAlign(ctx *session.Context, align model.BlockAlign, ids ...string) error {
	return nil
}

func (st *SmartTest) SetVerticalAlign(ctx *session.Context, align model.BlockVerticalAlign, ids ...string) error {
	return nil
}

func (st *SmartTest) SetLayout(ctx *session.Context, layout model.ObjectTypeLayout) error {
	return nil
}

func (st *SmartTest) SetRestrictions(r restriction.Restrictions) {
	st.TestRestrictions = r
}

func (st *SmartTest) Restrictions() restriction.Restrictions {
	return st.TestRestrictions
}

func (st *SmartTest) GetDocInfo() (DocInfo, error) {
	return DocInfo{
		Id: st.Id(),
	}, nil
}

func (st *SmartTest) AddHook(f smartblock.HookCallback, events ...smartblock.Hook) {
	return
}

func (st *SmartTest) HasRelation(s *state.State, key string) bool {
	for _, rel := range s.GetRelationLinks() {
		if rel.Key == key {
			return true
		}
	}
	return false
}

func (st *SmartTest) Relations(s *state.State) relationutils.Relations {
	return nil
}

func (st *SmartTest) DefaultObjectTypeUrl() string {
	return ""
}

func (st *SmartTest) TemplateCreateFromObjectState() (*state.State, error) {
	return st.Doc.NewState().Copy(), nil
}

func (st *SmartTest) AddRelationLinks(ctx *session.Context, relationKeys ...string) (err error) {
	if st.meta == nil {
		st.meta = &core.SmartBlockMeta{
			Details: &types.Struct{
				Fields: make(map[string]*types.Value),
			}}
	}
	for _, key := range relationKeys {
		st.Doc.(*state.State).AddRelationLinks(&model.RelationLink{
			Key:    key,
			Format: 0, // todo
		})
	}
	return nil
}

func (st *SmartTest) AddRelationLinksToState(s *state.State, relationKeys ...string) (err error) {
	return st.AddRelationLinks(nil, relationKeys...)
}

func (st *SmartTest) CheckSubscriptions() (changed bool) {
	return false
}

func (st *SmartTest) RefreshLocalDetails(ctx *session.Context) error {
	return nil
}

func (st *SmartTest) RemoveExtraRelations(ctx *session.Context, relationKeys []string) (err error) {
	return nil
}

func (st *SmartTest) SetObjectTypes(ctx *session.Context, objectTypes []string) (err error) {
	return nil
}

func (st *SmartTest) DisableLayouts() {
	return
}

func (st *SmartTest) SendEvent(msgs []*pb.EventMessage) {
	return
}

func (st *SmartTest) SetDetails(ctx *session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) (err error) {
	if st.meta == nil {
		st.meta = &core.SmartBlockMeta{
			Details: &types.Struct{
				Fields: make(map[string]*types.Value),
			}}
	}
	for _, d := range details {
		st.meta.Details.Fields[d.Key] = d.Value
	}
	st.Doc.(*state.State).SetDetails(st.meta.Details)
	return
}

func (st *SmartTest) Init(ctx *smartblock.InitContext) (err error) {
	return
}

func (st *SmartTest) Id() string {
	return st.id
}

func (st *SmartTest) Type() model.SmartBlockType {
	return model.SmartBlockType_Page
}

func (st *SmartTest) Show(*session.Context) (obj *model.ObjectView, err error) {
	return
}

func (st *SmartTest) SetEventFunc(f func(e *pb.Event)) {
}

func (st *SmartTest) Apply(s *state.State, flags ...smartblock.ApplyFlag) (err error) {
	var sendEvent, addHistory, checkRestrictions = true, true, true

	for _, f := range flags {
		switch f {
		case smartblock.NoEvent:
			sendEvent = false
		case smartblock.NoHistory:
			addHistory = false
		case smartblock.NoRestrictions:
			checkRestrictions = false
		}
	}

	if checkRestrictions {
		if err = s.CheckRestrictions(); err != nil {
			return
		}
	}

	msgs, act, err := state.ApplyState(s, true)
	if err != nil {
		return
	}

	if st.hist != nil && addHistory {
		st.hist.Add(act)
	}
	if sendEvent {
		st.Results.Events = append(st.Results.Events, msgs)
	}
	st.Results.Applies = append(st.Results.Applies, st.Blocks())
	return
}

func (st *SmartTest) History() undo.History {
	return st.hist
}

func (st *SmartTest) Anytype() core.Service {
	return st.anytype
}

func (st *SmartTest) MockAnytype() *testMock.MockService {
	return st.anytype
}

func (st *SmartTest) AddBlock(b simple.Block) *SmartTest {
	st.Doc.(*state.State).Add(b)
	return st
}

func (st *SmartTest) ResetToVersion(s *state.State) (err error) {
	return nil
}

func (st *SmartTest) FileRelationKeys(s *state.State) []string {
	return nil
}

func (st *SmartTest) ObjectClose() {
	st.SetEventFunc(nil)
}

func (st *SmartTest) Close() (err error) {
	return
}

func (st *SmartTest) SetObjectStore(os *testMock.MockObjectStore) {
	st.os = os
}

type Results struct {
	Events  [][]simple.EventMessage
	Applies [][]*model.Block
}
