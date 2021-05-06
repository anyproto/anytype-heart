package indexer_test

import (
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/core/recordsbatcher"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/clientds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockIndexer"
	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIndexer(t *testing.T) {
	t.Run("open/close", func(t *testing.T) {
		fx := newFixture(t)
		// should add all bundled relations to full text index
		defer fx.Close()
		defer fx.tearDown()

	})
	t.Run("indexMeta", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.Close()
		defer fx.tearDown()

		var (
			sbId = "sbId"
			sb   = testMock.NewMockSmartBlock(fx.ctrl)
			det  = &types.Struct{
				Fields: map[string]*types.Value{
					"key": pbtypes.String("value"),
				},
			}
			snaphot = &pb.Change{
				Snapshot: &pb.ChangeSnapshot{
					Data: &model.SmartBlockSnapshotBase{
						Details: det,
					},
				},
				Timestamp: time.Now().Unix(),
			}
			payload, _ = snaphot.Marshal()
			updatedCh  = make(chan struct{})
		)
		sb.EXPECT().ID().Return(sbId).AnyTimes()
		sb.EXPECT().Type().Return(smartblock.SmartBlockTypePage).AnyTimes()

		sb.EXPECT().GetLogs().Return(nil, nil)
		fx.anytype.EXPECT().GetBlock(sbId).Return(sb, nil)
		fx.objectStore.EXPECT().AddToIndexQueue(sbId)
		fx.objectStore.EXPECT().GetDetails(sbId)

		fx.objectStore.EXPECT().UpdateObjectDetails(sbId, gomock.Any(), gomock.Any()).DoAndReturn(func(id string, details *types.Struct, relations *model.Relations) (err error) {
			assert.Equal(t, "value", pbtypes.GetString(det, "key"))
			close(updatedCh)
			return
		})

		fx.rb.Add(core.SmartblockRecordWithThreadID{
			SmartblockRecordEnvelope: core.SmartblockRecordEnvelope{
				SmartblockRecord: core.SmartblockRecord{
					ID:      "snapshot",
					Payload: payload,
				},
			},
			ThreadID: sbId,
		})

		select {
		case <-updatedCh:
		case <-time.After(time.Second * 5):
			t.Errorf("index timeout")
		}
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
	fx.getSerach = mockIndexer.NewMockGetSearchInfo(fx.ctrl)
	fx.getSerach.EXPECT().Name().AnyTimes().Return("blockService")
	fx.getSerach.EXPECT().Init(gomock.Any())
	fx.objectStore = testMock.RegisterMockObjectStore(fx.ctrl, ta)

	fx.getSerach.EXPECT().GetSearchInfo(gomock.Any()).AnyTimes()

	for _, rk := range bundle.ListRelationsKeys() {
		fx.objectStore.EXPECT().GetDetails(addr.BundledRelationURLPrefix + rk.String())
		fx.objectStore.EXPECT().AddToIndexQueue(addr.BundledRelationURLPrefix + rk.String())

	}
	for _, ok := range bundle.ListTypesKeys() {
		fx.objectStore.EXPECT().GetDetails(ok.URL())
		fx.objectStore.EXPECT().AddToIndexQueue(ok.URL())
	}
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
	}).Times(1)

	fx.Indexer = indexer.New()

	rootPath, err := ioutil.TempDir(os.TempDir(), "anytype_*")
	require.NoError(t, err)
	cfg := config.DefaultConfig
	cfg.NewAccount = true
	ta.With(&cfg).With(wallet.NewWithRepoPathAndKeys(rootPath, nil, nil)).With(clientds.New()).With(ftsearch.New()).With(fx.rb).With(fx.Indexer).With(fx.getSerach)
	require.NoError(t, ta.Start())
	return fx
}

type fixture struct {
	indexer.Indexer
	ctrl        *gomock.Controller
	anytype     *testMock.MockService
	objectStore *testMock.MockObjectStore
	getSerach   *mockIndexer.MockGetSearchInfo
	ch          chan core.SmartblockRecordWithThreadID
	rb          recordsbatcher.RecordsBatcher
	ta          *testapp.TestApp
}

func (fx *fixture) tearDown() {
	fx.rb.(io.Closer).Close()
	fx.ta.Close()
	fx.ctrl.Finish()
}
