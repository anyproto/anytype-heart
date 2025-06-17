package smarttest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

func New(id string) *SmartTest {
	return &SmartTest{
		id:        id,
		Doc:       state.NewDoc(id, nil),
		hist:      undo.NewHistory(0),
		hooksOnce: map[string]struct{}{},
		sbType:    coresb.SmartBlockTypePage,
	}
}

func NewWithTree(id string, tree objecttree.ObjectTree) *SmartTest {
	return &SmartTest{
		id:         id,
		Doc:        state.NewDoc(id, nil),
		hist:       undo.NewHistory(0),
		hooksOnce:  map[string]struct{}{},
		sbType:     coresb.SmartBlockTypePage,
		objectTree: tree,
	}
}

var _ smartblock.SmartBlock = &SmartTest{}

type SmartTest struct {
	sync.Mutex
	state.Doc
	Results          Results
	id               string
	hist             undo.History
	TestRestrictions restriction.Restrictions
	App              *app.App
	objectTree       objecttree.ObjectTree
	isDeleted        bool
	os               *spaceindex.StoreFixture
	space            smartblock.Space

	// Rudimentary hooks
	hooks     []smartblock.HookCallback
	hooksOnce map[string]struct{}
	sbType    coresb.SmartBlockType
	spaceId   string
}

func (st *SmartTest) SpaceID() string { return st.spaceId }
func (st *SmartTest) SetSpaceId(spaceId string) {
	st.spaceId = spaceId
}
func (st *SmartTest) SetSpace(space smartblock.Space) {
	st.space = space
}

type stubSpace struct {
}

func (s *stubSpace) RefreshObjects(objectIds []string) (err error) {
	return nil
}

func (s *stubSpace) Id() string {
	return ""
}

func (s *stubSpace) TreeBuilder() objecttreebuilder.TreeBuilder {
	return nil
}

func (s *stubSpace) DerivedIDs() threads.DerivedSmartblockIds {
	return threads.DerivedSmartblockIds{}
}

func (s *stubSpace) GetRelationIdByKey(ctx context.Context, key domain.RelationKey) (id string, err error) {
	return
}

func (s *stubSpace) GetTypeIdByKey(ctx context.Context, key domain.TypeKey) (id string, err error) {
	return
}

func (s *stubSpace) DeriveObjectID(ctx context.Context, uniqueKey domain.UniqueKey) (id string, err error) {
	return
}

func (s *stubSpace) Do(objectId string, apply func(sb smartblock.SmartBlock) error) error {
	return nil
}

func (s *stubSpace) DoLockedIfNotExists(objectID string, proc func() error) error {
	return nil
}

func (s *stubSpace) TryRemove(objectId string) (bool, error) {
	return true, nil
}

func (s *stubSpace) IsPersonal() bool {
	return false
}

func (s *stubSpace) StoredIds() []string {
	return nil
}

func (st *SmartTest) Space() smartblock.Space {
	if st.space != nil {
		return st.space
	}
	return &stubSpace{}
}

func (st *SmartTest) EnabledRelationAsDependentObjects() {
	return
}

func (st *SmartTest) IsLocked() bool {
	return false
}

func (st *SmartTest) Locked() bool {
	return false
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
	return st.objectTree
}

func (st *SmartTest) Restrictions() restriction.Restrictions {
	return st.TestRestrictions
}

func (st *SmartTest) GetDocInfo() smartblock.DocInfo {
	return smartblock.DocInfo{
		Id:             st.Id(),
		Space:          st.Space(),
		SmartblockType: st.sbType,
		Heads:          []string{st.Id()},
	}
}

func (st *SmartTest) AddHook(f smartblock.HookCallback, events ...smartblock.Hook) {
	st.hooks = append(st.hooks, f)
	return
}

func (st *SmartTest) AddHookOnce(id string, f smartblock.HookCallback, events ...smartblock.Hook) {
	if _, ok := st.hooksOnce[id]; !ok {
		st.AddHook(f, events...)
		st.hooksOnce[id] = struct{}{}
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

func (st *SmartTest) AddRelationLinks(ctx session.Context, relationKeys ...domain.RelationKey) (err error) {
	for _, key := range relationKeys {
		st.Doc.(*state.State).AddRelationLinks(&model.RelationLink{
			Key:    key.String(),
			Format: 0, // todo
		})
	}
	return nil
}

func (st *SmartTest) AddRelationLinksToState(s *state.State, relationKeys ...domain.RelationKey) (err error) {
	return st.AddRelationLinks(nil, relationKeys...)
}

func (st *SmartTest) CheckSubscriptions() (changed bool) {
	return false
}

func (st *SmartTest) RefreshLocalDetails(ctx session.Context) error {
	return nil
}

func (st *SmartTest) RemoveExtraRelations(ctx session.Context, relationKeys []domain.RelationKey) (err error) {
	return nil
}

func (st *SmartTest) SetObjectTypes(objectTypes []domain.TypeKey) {
	st.Doc.(*state.State).SetObjectTypeKeys(objectTypes)
}

func (st *SmartTest) EnableLayouts() {
	return
}

func (st *SmartTest) IsLayoutsEnabled() bool {
	return false
}

func (st *SmartTest) SendEvent(msgs []*pb.EventMessage) {
	return
}

func (st *SmartTest) SetDetails(ctx session.Context, details []domain.Detail, showEvent bool) (err error) {
	dets := domain.NewDetails()
	for _, d := range details {
		dets.Set(d.Key, d.Value)
	}
	st.Doc.(*state.State).SetDetails(dets)
	return
}

func (st *SmartTest) UpdateDetails(ctx session.Context, update func(current *domain.Details) (*domain.Details, error)) (err error) {
	details := st.Doc.(*state.State).CombinedDetails()
	if details == nil {
		details = domain.NewDetails()
	}
	newDetails, err := update(details)
	if err != nil {
		return err
	}
	st.Doc.(*state.State).SetDetails(newDetails)
	return nil
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
	return st.sbType
}

func (st *SmartTest) SetType(sbType coresb.SmartBlockType) {
	st.sbType = sbType
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
		s.RemoveDetail(bundle.RelationKeyInternalFlags)
	}

	msgs, act, err := state.ApplyState(st.SpaceID(), s, true)
	if err != nil {
		return
	}

	if hooks {
		for _, h := range st.hooks {
			if err = h(smartblock.ApplyInfo{State: s, Changes: s.GetChanges()}); err != nil {
				return fmt.Errorf("exec hook: %w", err)
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

func (st *SmartTest) StateRebuild(d state.Doc) (err error) {
	d.(*state.State).SetParent(st.Doc.(*state.State))
	_, _, err = state.ApplyState(st.SpaceID(), d.(*state.State), false)
	return err
}

func (st *SmartTest) StateAppend(func(d state.Doc) (s *state.State, changes []*pb.ChangeContent, err error)) error {
	st.Results.IsStateAppendCalled = true
	return nil
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

func (st *SmartTest) Inner() smartblock.SmartBlock {
	return nil
}

func (st *SmartTest) ObjectCloseAllSessions() {
}

func (st *SmartTest) RegisterSession(session.Context) {

}

func (st *SmartTest) UniqueKey() domain.UniqueKey {
	return nil
}

func (st *SmartTest) Update(ctx session.Context, apply func(b simple.Block) error, blockIds ...string) (err error) {
	newState := st.NewState()
	for _, id := range blockIds {
		if bl := newState.Get(id); bl != nil {
			if err = apply(bl); err != nil {
				return err
			}
		}
	}
	return st.Apply(newState)
}

type Results struct {
	Events              [][]simple.EventMessage
	Applies             [][]*model.Block
	IsStateAppendCalled bool
}
