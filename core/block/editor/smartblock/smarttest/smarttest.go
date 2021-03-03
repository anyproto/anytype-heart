package smarttest

import (
	"fmt"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
)

func New(id string) *SmartTest {
	return &SmartTest{
		id:   id,
		Doc:  state.NewDoc(id, nil),
		hist: undo.NewHistory(0),
	}
}

func NewWithMeta(id string, ms meta.Service) *SmartTest {
	st := New(id)
	st.ms = ms
	return st
}

type SmartTest struct {
	Results Results
	anytype *testMock.MockService
	id      string
	hist    undo.History
	meta    *core.SmartBlockMeta
	ms      meta.Service
	sync.Mutex
	state.Doc
}

func (st *SmartTest) GetSearchInfo() (indexer.SearchInfo, error) {
	return indexer.SearchInfo{
		Id:      st.Id(),
		Title:   pbtypes.GetString(st.Details(), "name"),
		Snippet: st.Snippet(),
		Text:    st.Doc.SearchText(),
	}, nil
}

func (st *SmartTest) AddHook(f func(), events ...smartblock.Hook) {
	return
}

func (st *SmartTest) HasRelation(relationKey string) bool {
	return st.NewState().HasRelation(relationKey)
}

func (st *SmartTest) Relations() []*pbrelation.Relation {
	return nil
}

func (st *SmartTest) DefaultObjectTypeUrl() string {
	return ""
}

func (st *SmartTest) AddExtraRelationOption(ctx *state.Context, relationKey string, option pbrelation.RelationOption, showEvent bool) (*pbrelation.RelationOption, error) {
	rel := pbtypes.GetRelation(st.Relations(), relationKey)
	if rel == nil {
		return nil, fmt.Errorf("relation not found")
	}

	if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
		return nil, fmt.Errorf("incorrect relation format")
	}

	newOption, err := st.Doc.(*state.State).AddExtraRelationOption(*rel, option)
	if err != nil {
		return nil, err
	}

	return newOption, nil
}

func (st *SmartTest) CheckSubscriptions() (changed bool) {
	return false
}

func (st *SmartTest) UpdateExtraRelationOption(ctx *state.Context, relationKey string, option pbrelation.RelationOption, showEvent bool) error {
	for _, rel := range st.ExtraRelations() {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
			return fmt.Errorf("relation has incorrect format")
		}
		for i, opt := range rel.SelectDict {
			if opt.Id == option.Id {
				copy := pbtypes.CopyRelation(rel)
				copy.SelectDict[i] = &option
				st.Doc.(*state.State).SetExtraRelation(copy)

				return nil
			}
		}

		return fmt.Errorf("relation option not found")
	}

	return fmt.Errorf("relation not found")
}

func (st *SmartTest) DeleteExtraRelationOption(ctx *state.Context, relationKey string, optionId string, showEvent bool) error {
	for _, rel := range st.ExtraRelations() {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
			return fmt.Errorf("relation has incorrect format")
		}
		for i, opt := range rel.SelectDict {
			if opt.Id == optionId {
				copy := pbtypes.CopyRelation(rel)
				copy.SelectDict = append(rel.SelectDict[:i], rel.SelectDict[i+1:]...)
				st.Doc.(*state.State).SetExtraRelation(copy)
				return nil
			}
		}
		// todo: should we remove option and value from all objects within type?

		return fmt.Errorf("relation option not found")
	}

	return fmt.Errorf("relation not found")
}

func (st *SmartTest) AddExtraRelations(ctx *state.Context, relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error) {
	if st.meta == nil {
		st.meta = &core.SmartBlockMeta{
			Details: &types.Struct{
				Fields: make(map[string]*types.Value),
			}}
	}
	for _, d := range relations {
		if d.Key == "" {
			d.Key = bson.NewObjectId().Hex()
		}
		st.meta.Relations = append(st.meta.Relations, pbtypes.CopyRelation(d))
	}
	st.Doc.(*state.State).SetExtraRelations(st.meta.Relations)
	return st.meta.Relations, nil
}

func (st *SmartTest) UpdateExtraRelations(ctx *state.Context, relations []*pbrelation.Relation, createIfMissing bool) (err error) {
	if st.meta == nil {
		st.meta = &core.SmartBlockMeta{
			Details: &types.Struct{
				Fields: make(map[string]*types.Value),
			}}
	}
	for _, d := range relations {
		var found bool
		for i, rel := range st.meta.Relations {
			if rel.Key != d.Key {
				continue
			}
			found = true
			st.meta.Relations[i] = d
		}
		if !found && !createIfMissing {
			return fmt.Errorf("relation not found")
		}
	}

	st.Doc.(*state.State).SetExtraRelations(st.meta.Relations)
	return nil
}

func (st *SmartTest) RemoveExtraRelations(ctx *state.Context, relationKeys []string) (err error) {
	return nil
}

func (st *SmartTest) SetObjectTypes(ctx *state.Context, objectTypes []string) (err error) {
	return nil
}

func (st *SmartTest) DisableLayouts() {
	return
}

func (st *SmartTest) SendEvent(msgs []*pb.EventMessage) {
	return
}

func (st *SmartTest) SetDetails(ctx *state.Context, details []*pb.RpcBlockSetDetailsDetail, showEvent bool) (err error) {
	if st.meta == nil {
		st.meta = &core.SmartBlockMeta{
			Relations: st.ExtraRelations(),
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

func (st *SmartTest) Init(_ source.Source, _ bool, _ []string) (err error) {
	return
}

func (st *SmartTest) Id() string {
	return st.id
}

func (st *SmartTest) Type() pb.SmartBlockType {
	return pb.SmartBlockType_Page
}

func (st *SmartTest) Show(*state.Context) (err error) {
	return
}

func (st *SmartTest) Meta() *core.SmartBlockMeta {
	return st.meta
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

func (st *SmartTest) Anytype() anytype.Service {
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

func (st *SmartTest) MetaService() meta.Service {
	return st.ms
}

func (st *SmartTest) FileRelationKeys() []string {
	return nil
}

func (st *SmartTest) BlockClose() {
	st.SetEventFunc(nil)
}

func (st *SmartTest) Close() (err error) {
	return
}

type Results struct {
	Events  [][]simple.EventMessage
	Applies [][]*model.Block
}
