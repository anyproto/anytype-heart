package indexer_test

import (
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
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
		defer fx.tearDown()
		defer fx.Close()

	})
	t.Run("indexMeta", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
		defer fx.Close()

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
		sb.EXPECT().GetLogs().Return(nil, nil)
		fx.anytype.EXPECT().GetBlock(sbId).Return(sb, nil)
		fx.objectStore.EXPECT().AddToIndexQueue(sbId)
		fx.objectStore.EXPECT().UpdateObject(sbId, gomock.Any(), gomock.Any(), nil, "").DoAndReturn(func(id string, details *types.Struct, relations *pbrelation.Relations, links []string, snippet string) (err error) {
			assert.Equal(t, "value", pbtypes.GetString(det, "key"))
			close(updatedCh)
			return
		})

		fx.ch <- core.SmartblockRecordWithThreadID{
			SmartblockRecordEnvelope: core.SmartblockRecordEnvelope{
				SmartblockRecord: core.SmartblockRecord{
					ID:      "snapshot",
					Payload: payload,
				},
			},
			ThreadID: sbId,
		}

		select {
		case <-updatedCh:
		case <-time.After(time.Second * 5):
			t.Errorf("index timeout")
		}
	})
}

func newFixture(t *testing.T) *fixture {
	var err error
	fx := &fixture{
		ctrl: gomock.NewController(t),
	}
	fx.getSerach = mockIndexer.NewMockGetSearchInfo(fx.ctrl)
	fx.anytype = testMock.NewMockService(fx.ctrl)
	fx.objectStore = testMock.NewMockObjectStore(fx.ctrl)
	fx.objectStore.EXPECT().FTSearch().Return(nil).AnyTimes()
	fx.anytype.EXPECT().ObjectStore().Return(fx.objectStore).AnyTimes()
	fx.ch = make(chan core.SmartblockRecordWithThreadID)
	fx.anytype.EXPECT().SubscribeForNewRecords().Return(fx.ch, func() {
		close(fx.ch)
	}, nil)
	fx.Indexer, err = indexer.NewIndexer(fx.anytype, fx.getSerach)
	require.NoError(t, err)
	return fx
}

type fixture struct {
	indexer.Indexer
	ctrl        *gomock.Controller
	anytype     *testMock.MockService
	objectStore *testMock.MockObjectStore
	getSerach   *mockIndexer.MockGetSearchInfo
	ch          chan core.SmartblockRecordWithThreadID
}

func (fx *fixture) tearDown() {
	fx.ctrl.Finish()
}
