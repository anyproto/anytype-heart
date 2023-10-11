package smarttest

import (
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/system_object/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/testMock"
)

func New(id string) *SmartTest {
	return &SmartTest{
		id:        id,
		Doc:       state.NewDoc(id, nil),
		hist:      undo.NewHistory(0),
		hooksOnce: map[string]struct{}{},
	}
}

var _ smartblock.SmartBlock = &SmartTest{}

type SmartTest struct {
	Results          Results
	anytype          *testMock.MockService
	id               string
	hist             undo.History
	TestRestrictions restriction.Restrictions
	App              *app.App
	sync.Mutex
	state.Doc
	isDeleted bool
	os        *testMock.MockObjectStore

	// Rudimentary hooks
	hooks     []smartblock.HookCallback
	hooksOnce map[string]struct{}
}

func (st *SmartTest) SpaceID() string { return "" }

func (st *SmartTest) EnabledRelationAsDependentObjects() {
	return
}

func (st *SmartTest) IsLocked() bool {
	return false
}

func (st *SmartTest) Locked() bool {
	return false
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

func (st *SmartTest) SetAlign(ctx session.Context, align model.BlockAlign, ids ...string) error {
	return nil
}

func (st *SmartTest) SetVerticalAlign(ctx session.Context, align model.BlockVerticalAlign, ids ...string) error {
	return nil
}

func (st *SmartTest) SetLayout(ctx session.Context, layout model.ObjectTypeLayout) error {
	return nil
}

func (st *SmartTest) SetLocker(locker smartblock.Locker) {}

func (st *SmartTest) Tree() objecttree.ObjectTree {
	return nil
}

func (st *SmartTest) SetRestrictions(r restriction.Restrictions) {
	st.TestRestrictions = r
}

func (st *SmartTest) Restrictions() restriction.Restrictions {
	return st.TestRestrictions
}

func (st *SmartTest) GetDocInfo() smartblock.DocInfo {
	return smartblock.DocInfo{
		Id: st.Id(),
	}
}

func (st *SmartTest) AddHook(f smartblock.HookCallback, events ...smartblock.Hook) {
	st.hooks = append(st.hooks, f)
	return
}

func (sb *SmartTest) AddHookOnce(id string, f smartblock.HookCallback, events ...smartblock.Hook) {
	if _, ok := sb.hooksOnce[id]; !ok {
		sb.AddHook(f, events...)
		sb.hooksOnce[id] = struct{}{}
	}
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

func (st *SmartTest) AddRelationLinks(ctx session.Context, relationKeys ...string) (err error) {
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

func (st *SmartTest) RefreshLocalDetails(ctx session.Context) error {
	return nil
}

func (st *SmartTest) RemoveExtraRelations(ctx session.Context, relationKeys []string) (err error) {
	return nil
}

func (st *SmartTest) SetObjectTypes(ctx session.Context, objectTypes []string) (err error) {
	return nil
}

func (st *SmartTest) DisableLayouts() {
	return
}

func (st *SmartTest) SendEvent(msgs []*pb.EventMessage) {
	return
}

func (st *SmartTest) SetDetails(ctx session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) (err error) {
	dets := &types.Struct{Fields: map[string]*types.Value{}}
	for _, d := range details {
		dets.Fields[d.Key] = d.Value
	}
	st.Doc.(*state.State).SetDetails(dets)
	return
}

func (st *SmartTest) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.State == nil {
		ctx.State = st.NewState()
	}
	return
}

func (st *SmartTest) Id() string {
	return st.id
}

func (st *SmartTest) Type() coresb.SmartBlockType {
	return coresb.SmartBlockTypePage
}

func (st *SmartTest) Show() (obj *model.ObjectView, err error) {
	return
}

func (st *SmartTest) SetEventFunc(f func(e *pb.Event)) {
}

func (st *SmartTest) Apply(s *state.State, flags ...smartblock.ApplyFlag) (err error) {
	var sendEvent, addHistory, checkRestrictions, hooks, keepInternalFlags = true, true, true, true, false

	for _, f := range flags {
		switch f {
		case smartblock.NoEvent:
			sendEvent = false
		case smartblock.NoHistory:
			addHistory = false
		case smartblock.NoRestrictions:
			checkRestrictions = false
		case smartblock.NoHooks:
			hooks = false
		case smartblock.KeepInternalFlags:
			keepInternalFlags = true
		}
	}

	if checkRestrictions {
		if err = s.CheckRestrictions(); err != nil {
			return
		}
	}

	if !keepInternalFlags {
		s.RemoveDetail(bundle.RelationKeyInternalFlags.String())
	}

	msgs, act, err := state.ApplyState(s, true)
	if err != nil {
		return
	}

	if hooks {
		for _, h := range st.hooks {
			if err = h(smartblock.ApplyInfo{State: s, Changes: s.GetChanges()}); err != nil {
				return fmt.Errorf("exec hook: %s", err)
			}
		}
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

func (st *SmartTest) ObjectClose(ctx session.Context) {
	st.SetEventFunc(nil)
}

func (st *SmartTest) Close() (err error) {
	return
}

func (st *SmartTest) TryClose(objectTTL time.Duration) (res bool, err error) {
	return
}

func (st *SmartTest) SetObjectStore(os *testMock.MockObjectStore) {
	st.os = os
}

func (st *SmartTest) Inner() smartblock.SmartBlock {
	return nil
}

func (st *SmartTest) ObjectCloseAllSessions() {
}

func (st *SmartTest) RegisterSession(session.Context) {

}

type Results struct {
	Events  [][]simple.EventMessage
	Applies [][]*model.Block
}
