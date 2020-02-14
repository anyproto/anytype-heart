package block

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func newServiceFixture(t *testing.T, accountId string) *serviceFixture {
	ctrl := gomock.NewController(t)
	anytype := testMock.NewMockAnytype(ctrl)
	fx := &serviceFixture{
		t:       t,
		ctrl:    ctrl,
		anytype: anytype,
	}
	fx.Service = NewService(accountId, anytype, fx.sendEvent)
	return fx
}

type serviceFixture struct {
	Service
	t       *testing.T
	ctrl    *gomock.Controller
	anytype *testMock.MockAnytype
	events  []*pb.Event
}

func (fx *serviceFixture) sendEvent(e *pb.Event) {
	fx.events = append(fx.events, e)
}

func (fx *serviceFixture) newMockBlockWithContent(id string, content model.IsBlockContent, childrenIds []string, db map[string]core.BlockVersion) (b *blockWrapper, v *testMock.MockBlockVersion) {
	if db == nil {
		db = make(map[string]core.BlockVersion)
	}
	v = fx.newMockVersion(&model.Block{
		Id:          id,
		Content:     content,
		ChildrenIds: childrenIds,
	})
	v.EXPECT().DependentBlocks().AnyTimes().Return(db)
	b = &blockWrapper{MockBlock: testMock.NewMockBlock(fx.ctrl)}
	b.EXPECT().GetId().AnyTimes().Return(id)
	b.EXPECT().GetCurrentVersion().AnyTimes().Return(v, nil)
	fx.anytype.EXPECT().PredefinedBlockIds().AnyTimes().Return(core.PredefinedBlockIds{Home: "realHomeId"})
	return
}

func (fx *serviceFixture) newMockVersion(m *model.Block) (v *testMock.MockBlockVersion) {
	v = testMock.NewMockBlockVersion(fx.ctrl)
	v.EXPECT().Model().AnyTimes().Return(m)
	return
}

func (fx *serviceFixture) tearDown() {
	require.NoError(fx.t, fx.Close())
}

type blockWrapper struct {
	*testMock.MockBlock
	clientEventsChan          chan<- proto.Message
	blockVersionsChan         chan<- []core.BlockVersion
	blockMetaChan             chan<- core.BlockVersionMeta
	blockMetaChanSubscribed   chan struct{}
	cancelClientEventsCalled  bool
	cancelBlockVersionsCalled bool
	cancelBlockMetaCalled     bool
	saveBlocksWriter          func(m ...*model.Block)
	newBlockHandler           func(m model.Block) (core.Block, error)

	m sync.Mutex
}

func (bw *blockWrapper) SubscribeClientEvents(ch chan<- proto.Message) (func(), error) {
	bw.m.Lock()
	defer bw.m.Unlock()
	bw.clientEventsChan = ch
	return func() {
		bw.m.Lock()
		defer bw.m.Unlock()
		bw.cancelClientEventsCalled = true
		close(ch)
	}, nil
}

func (bw *blockWrapper) SubscribeNewVersionsOfBlocks(v string, f bool, ch chan<- []core.BlockVersion) (func(), error) {
	bw.m.Lock()
	defer bw.m.Unlock()
	bw.blockVersionsChan = ch
	return func() {
		bw.m.Lock()
		defer bw.m.Unlock()
		bw.cancelBlockVersionsCalled = true
		close(ch)
	}, nil
}

func (bw *blockWrapper) SubscribeMetaOfNewVersionsOfBlock(sinceVersionId string, includeSinceVersion bool, ch chan<- core.BlockVersionMeta) (cancelFunc func(), err error) {
	bw.m.Lock()
	defer bw.m.Unlock()
	bw.blockMetaChan = ch
	if bw.blockMetaChanSubscribed != nil {
		close(bw.blockMetaChanSubscribed)
	}
	return func() {
		bw.m.Lock()
		defer bw.m.Unlock()
		bw.cancelBlockMetaCalled = true
		close(ch)
	}, nil
}

func (bw *blockWrapper) AddVersions(m []*model.Block) (vers []core.BlockVersion, err error) {
	if bw.saveBlocksWriter != nil {
		bw.saveBlocksWriter(m...)
		return make([]core.BlockVersion, len(m)), nil
	}
	return bw.MockBlock.AddVersions(m)
}

func (bw *blockWrapper) NewBlock(m model.Block) (core.Block, error) {
	if bw.newBlockHandler != nil {
		return bw.newBlockHandler(m)
	}
	return bw.MockBlock.NewBlock(m)
}

type matcher struct {
	name string
	f    func(x interface{}) bool
}

func (m *matcher) Matches(x interface{}) bool {
	return m.f(x)
}

func (m *matcher) String() string {
	return m.name
}

func newStateFixture(t *testing.T) *stateFixture {
	ctrl := gomock.NewController(t)
	block := &testBlock{
		MockBlock: testMock.NewMockBlock(ctrl),
	}
	block.EXPECT().GetId().AnyTimes().Return("root")
	sb := &commonSmart{
		block:    block,
		versions: make(map[string]simple.Block),
		s:        &service{},
	}
	sb.versions[block.GetId()] = simple.New(&model.Block{
		Id:          "root",
		ChildrenIds: []string{},
		Content: &model.BlockContentOfPage{
			Page: &model.BlockContentPage{},
		},
	})
	fx := &stateFixture{
		block: block,
		state: sb.newState(),
		sb:    sb,
		ctrl:  ctrl,
	}
	fx.block.fx = fx
	return fx
}

type stateFixture struct {
	*state
	block *testBlock
	sb    *commonSmart
	ctrl  *gomock.Controller
	saved []*model.Block
}

func (fx *stateFixture) Finish() {
	fx.ctrl.Finish()
}

type testBlock struct {
	fx *stateFixture
	*testMock.MockBlock
}

func (tb *testBlock) AddVersions(vers []*model.Block) ([]core.BlockVersion, error) {
	tb.fx.saved = vers
	return nil, nil
}

func newPageFixture(t *testing.T, blocks ...*model.Block) *pageFixture {
	serviceFx := newServiceFixture(t, "testAccountId")
	pageFx := &pageFixture{}
	pageFx.ctrl = serviceFx.ctrl
	pageFx.serviceFx = serviceFx
	pageFx.savedBlocks = make(map[string]*model.Block)
	pageFx.pageId = fmt.Sprint(rand.Int63())
	db := make(map[string]core.BlockVersion)
	childrenIds := []string{}
	for _, b := range blocks {
		db[b.Id] = serviceFx.newMockVersion(b)
		childrenIds = append(childrenIds, b.Id)
	}
	for _, b := range blocks {
		for _, cid := range b.ChildrenIds {
			childrenIds = removeFromSlice(childrenIds, cid)
		}
	}
	pageFx.block, _ = serviceFx.newMockBlockWithContent(pageFx.pageId, &model.BlockContentOfPage{}, childrenIds, db)
	pageFx.block.saveBlocksWriter = func(ms ...*model.Block) {
		for _, m := range ms {
			pageFx.savedBlocks[m.Id] = m
		}
	}
	pageFx.block.newBlockHandler = func(m model.Block) (block core.Block, err error) {
		newId := fmt.Sprint(rand.Int63())
		mb := testMock.NewMockBlock(pageFx.ctrl)
		mb.EXPECT().GetId().AnyTimes().Return(newId)
		return mb, nil
	}
	pageFx.block.EXPECT().Close()

	serviceFx.anytype.EXPECT().GetBlockWithBatcher(pageFx.pageId).Return(pageFx.block, nil)

	require.NoError(t, serviceFx.OpenBlock(pageFx.pageId))
	pageFx.page = serviceFx.Service.(*service).openedBlocks[pageFx.pageId].smartBlock.(*page)
	return pageFx
}

type pageFixture struct {
	*page
	pageId    string
	ctrl      *gomock.Controller
	serviceFx *serviceFixture
	block     *blockWrapper
	// all saved blocks will be here
	savedBlocks map[string]*model.Block
}

func (fx *pageFixture) tearDown() {
	fx.serviceFx.tearDown()
}
