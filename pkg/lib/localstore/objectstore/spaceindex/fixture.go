package spaceindex

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/oldstore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var ctx = context.Background()

type StoreFixture struct {
	*dsObjectStore
}

const spaceName = "space1"

type detailsFromId struct {
}

func (d *detailsFromId) DetailsFromIdBasedSource(id string) (*types.Struct, error) {
	return nil, fmt.Errorf("not found")
}

type dummyFulltextQueue struct {
	lock        sync.Mutex
	idsPerSpace map[string][]string
}

func (q *dummyFulltextQueue) RemoveIdsFromFullTextQueue(ids []string) error {
	q.lock.Lock()
	defer q.lock.Unlock()
	for spaceId, spaceIds := range q.idsPerSpace {
		q.idsPerSpace[spaceId] = lo.Filter(spaceIds, func(id string, _ int) bool {
			return !lo.Contains(ids, id)
		})
	}
	return nil
}

func (q *dummyFulltextQueue) AddToIndexQueue(ctx context.Context, ids ...domain.FullID) error {
	q.lock.Lock()
	defer q.lock.Unlock()
	for _, id := range ids {
		if idsForSpace, ok := q.idsPerSpace[id.SpaceID]; ok {
			if !lo.Contains(idsForSpace, id.ObjectID) {
				q.idsPerSpace[id.SpaceID] = append(idsForSpace, id.ObjectID)
			}
		} else {
			q.idsPerSpace[id.SpaceID] = []string{id.ObjectID}
		}
	}
	return nil
}

func (q *dummyFulltextQueue) ListIdsFromFullTextQueue(spaceId string, limit int) ([]string, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if ids, ok := q.idsPerSpace[spaceId]; !ok {
		return nil, nil
	} else {
		if limit > len(ids) {
			limit = len(ids)
		}
		return ids[:limit], nil
	}
}

func NewStoreFixture(t testing.TB) *StoreFixture {
	walletService := mock_wallet.NewMockWallet(t)
	walletService.EXPECT().Name().Return(wallet.CName).Maybe()
	walletService.EXPECT().RepoPath().Return(t.TempDir())

	fullText := ftsearch.TantivyNew()
	testApp := &app.App{}

	dataStore, err := datastore.NewInMemory()
	require.NoError(t, err)

	testApp.Register(dataStore)
	testApp.Register(walletService)
	err = fullText.Init(testApp)
	require.NoError(t, err)
	err = fullText.Run(context.Background())
	require.NoError(t, err)

	oldStore := oldstore.New()
	err = oldStore.Init(testApp)
	require.NoError(t, err)

	s := New(context.Background(), "test", Deps{
		DbPath:         filepath.Join(t.TempDir(), "test.db"),
		Fts:            fullText,
		OldStore:       oldStore,
		SourceService:  &detailsFromId{},
		SubManager:     &SubscriptionManager{},
		AnyStoreConfig: nil,
		FulltextQueue:  &dummyFulltextQueue{idsPerSpace: map[string][]string{}},
	})
	return &StoreFixture{
		dsObjectStore: s.(*dsObjectStore),
	}
}

type TestObject map[domain.RelationKey]*types.Value

func generateObjectWithRandomID() TestObject {
	id := fmt.Sprintf("%d", rand.Int())
	return TestObject{
		bundle.RelationKeyId:   pbtypes.String(id),
		bundle.RelationKeyName: pbtypes.String("name" + id),
	}
}

func makeObjectWithName(id string, name string) TestObject {
	return TestObject{
		bundle.RelationKeyId:      pbtypes.String(id),
		bundle.RelationKeyName:    pbtypes.String(name),
		bundle.RelationKeySpaceId: pbtypes.String(spaceName),
	}
}

func makeObjectWithNameAndDescription(id string, name string, description string) TestObject {
	return TestObject{
		bundle.RelationKeyId:          pbtypes.String(id),
		bundle.RelationKeyName:        pbtypes.String(name),
		bundle.RelationKeyDescription: pbtypes.String(description),
		bundle.RelationKeySpaceId:     pbtypes.String(spaceName),
	}
}

func makeDetails(fields TestObject) *types.Struct {
	f := map[string]*types.Value{}
	for k, v := range fields {
		f[string(k)] = v
	}
	return &types.Struct{Fields: f}
}

func (fx *StoreFixture) AddObjects(t testing.TB, objects []TestObject) {
	for _, obj := range objects {
		id := obj[bundle.RelationKeyId].GetStringValue()
		require.NotEmpty(t, id)
		err := fx.UpdateObjectDetails(context.Background(), id, makeDetails(obj))
		require.NoError(t, err)
	}
}
