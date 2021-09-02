package indexer_test

import (
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/core/recordsbatcher"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/clientds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockBuiltinTemplate"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockDoc"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockStatus"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestNewIndexer(t *testing.T) {
	t.Run("open/close", func(t *testing.T) {
		fx := newFixture(t)
		// should add all bundled relations to full text index
		defer fx.Close()
		defer fx.tearDown()

	})
}

func newFixture(t *testing.T) *fixture {

	ta := testapp.New()
	rb := recordsbatcher.New()

	fx := &fixture{
		ctrl: gomock.NewController(t),
		ta:   ta,
		rb:   rb,
	}

	fx.anytype = testMock.RegisterMockAnytype(fx.ctrl, ta)
	fx.docService = mockDoc.NewMockService(fx.ctrl)
	fx.docService.EXPECT().Name().AnyTimes().Return(doc.CName)
	fx.docService.EXPECT().Init(gomock.Any())
	fx.docService.EXPECT().Run()
	fx.anytype.EXPECT().PredefinedBlocks()
	fx.docService.EXPECT().Close().AnyTimes()
	fx.objectStore = testMock.RegisterMockObjectStore(fx.ctrl, ta)

	fx.docService.EXPECT().GetDocInfo(gomock.Any(), gomock.Any()).Return(doc.DocInfo{State: state.NewDoc("", nil).(*state.State)}, nil).AnyTimes()
	fx.docService.EXPECT().OnWholeChange(gomock.Any())
	fx.objectStore.EXPECT().GetDetails(addr.AnytypeProfileId)
	fx.objectStore.EXPECT().AddToIndexQueue(addr.AnytypeProfileId)

	for _, rk := range bundle.ListRelationsKeys() {
		fx.objectStore.EXPECT().GetDetails(addr.BundledRelationURLPrefix + rk.String())
		fx.objectStore.EXPECT().AddToIndexQueue(addr.BundledRelationURLPrefix + rk.String())

	}
	for _, ok := range bundle.ListTypesKeys() {
		fx.objectStore.EXPECT().GetDetails(ok.URL())
		fx.objectStore.EXPECT().AddToIndexQueue(ok.URL())
	}
	fx.anytype.EXPECT().ProfileID().AnyTimes()
	fx.objectStore.EXPECT().GetDetails("_anytype_profile")
	fx.objectStore.EXPECT().AddToIndexQueue("_anytype_profile")
	fx.objectStore.EXPECT().FTSearch().Return(nil).AnyTimes()
	fx.objectStore.EXPECT().IndexForEach(gomock.Any()).Times(1)
	fx.objectStore.EXPECT().CreateObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	fx.anytype.EXPECT().ObjectStore().Return(fx.objectStore).AnyTimes()
	fx.objectStore.EXPECT().SaveChecksums(&model.ObjectStoreChecksums{
		BundledObjectTypes:         bundle.TypeChecksum,
		BundledRelations:           bundle.RelationChecksum,
		BundledLayouts:             "",
		ObjectsForceReindexCounter: indexer.ForceThreadsObjectsReindexCounter,
		FilesForceReindexCounter:   indexer.ForceFilesReindexCounter,
		IdxRebuildCounter:          indexer.ForceIdxRebuildCounter,
		FulltextRebuild:            indexer.ForceFulltextIndexCounter,
		BundledObjects:             indexer.ForceBundledObjectsReindexCounter,
	}).Times(1)

	fx.Indexer = indexer.New()

	rootPath, err := ioutil.TempDir(os.TempDir(), "anytype_*")
	require.NoError(t, err)
	cfg := config.DefaultConfig
	cfg.NewAccount = true
	ta.With(&cfg).With(wallet.NewWithRepoPathAndKeys(rootPath, nil, nil)).
		With(clientds.New()).
		With(ftsearch.New()).
		With(fx.rb).
		With(fx.Indexer).
		With(fx.docService).
		With(source.New())
	mockStatus.RegisterMockStatus(fx.ctrl, ta)
	mockBuiltinTemplate.RegisterMockBuiltinTemplate(fx.ctrl, ta).EXPECT().Hash().AnyTimes()
	require.NoError(t, ta.Start())
	return fx
}

type fixture struct {
	indexer.Indexer
	ctrl        *gomock.Controller
	anytype     *testMock.MockService
	objectStore *testMock.MockObjectStore
	docService  *mockDoc.MockService
	ch          chan core.SmartblockRecordWithThreadID
	rb          recordsbatcher.RecordsBatcher
	ta          *testapp.TestApp
}

func (fx *fixture) tearDown() {
	fx.rb.(io.Closer).Close()
	fx.ta.Close()
	fx.ctrl.Finish()
}
