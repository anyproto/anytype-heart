package subscription

import (
	"testing"

	"github.com/cheggaaa/mb"
	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
)

func TestCollections(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	updateCh := make(chan []string)
	store := testMock.NewMockObjectStore(ctrl)
	store.EXPECT().QueryById([]string{"1", "2", "3"}).Return([]database.Record{
		{Details: &types.Struct{Fields: map[string]*types.Value{
			"id":   pbtypes.String("1"),
			"name": pbtypes.String("1"),
		}}},
		{Details: &types.Struct{Fields: map[string]*types.Value{
			"id":   pbtypes.String("2"),
			"name": pbtypes.String("2"),
		}}},
		{Details: &types.Struct{Fields: map[string]*types.Value{
			"id":   pbtypes.String("3"),
			"name": pbtypes.String("3"),
		}}},
	}, nil)
	eventsCh := make(chan *pb.Event)
	cache := newCache()
	svc := &service{
		recBatch:    mb.New(0),
		objectStore: store,
		cache:       cache,
		collectionService: &collectionServiceMock{
			updateCh: updateCh,
		},
		sendEvent: func(e *pb.Event) {
			eventsCh <- e
		},
		ctxBuf:        &opCtx{c: cache},
		subscriptions: map[string]subscription{},
	}
	go svc.recordsHandler()

	sub, err := svc.newCollectionSub("sub", "collection", []string{"id", "name"}, nil, nil, 10, 0)
	require.NoError(t, err)

	svc.subscriptions["sub"] = sub

	updateCh <- []string{"1", "2", "3"}

	for e := range eventsCh {
		t.Log(e)
	}
	sub.close()
}
