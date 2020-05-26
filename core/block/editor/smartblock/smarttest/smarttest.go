package smarttest

import (
	"sync"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
)

func New(id string) *SmartTest {
	return &SmartTest{
		id:   id,
		Doc:  state.NewDoc(id, nil),
		hist: history.NewHistory(0),
	}
}

func NewWithAnytype(id string, ctrl *gomock.Controller) *SmartTest {
	st := New(id)
	st.anytype = testMock.NewMockService(ctrl)
	return st
}

type SmartTest struct {
	Results Results
	anytype *testMock.MockService
	id      string
	hist    history.History
	meta    *core.SmartBlockMeta
	sync.Mutex
	state.Doc
}

func (st *SmartTest) SetDetails(details []*pb.RpcBlockSetDetailsDetail) (err error) {
	if st.meta == nil {
		st.meta = &core.SmartBlockMeta{Details: &types.Struct{
			Fields: make(map[string]*types.Value),
		}}
	}
	for _, d := range details {
		st.meta.Details.Fields[d.Key] = d.Value
	}
	return
}

func (st *SmartTest) Init(_ source.Source) (err error) {
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
	var sendEvent, addHistory = true, true
	msgs, act, err := state.ApplyState(s)
	if err != nil {
		return
	}

	for _, f := range flags {
		switch f {
		case smartblock.NoEvent:
			sendEvent = false
		case smartblock.NoHistory:
			addHistory = false
		}
	}

	if st.hist != nil && addHistory {
		st.hist.Add(act)
	}
	if sendEvent {
		st.Results.Events = append(st.Results.Events, &pb.Event{
			Messages: msgs,
		})
	}
	st.Results.Applies = append(st.Results.Applies, st.Blocks())
	return
}

func (st *SmartTest) History() history.History {
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

func (st *SmartTest) Close() (err error) {
	return
}

type Results struct {
	Events  []*pb.Event
	Applies [][]*model.Block
}
