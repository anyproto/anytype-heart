package spaceindex

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
)

var ctx = context.Background()

type StoreFixture struct {
	*dsObjectStore
}

const spaceName = "space1"

type detailsFromId struct {
}

func (d *detailsFromId) DetailsFromIdBasedSource(id domain.FullID) (*domain.Details, error) {
	return nil, fmt.Errorf("not found")
}

type dummyFulltextQueue struct {
	lock sync.Mutex
	ids  []string
}

func (q *dummyFulltextQueue) RemoveIdsFromFullTextQueue(ids []string) error {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.ids = lo.Without(q.ids, ids...)
	return nil
}

func (q *dummyFulltextQueue) AddToIndexQueue(ctx context.Context, ids ...string) error {
	q.lock.Lock()
	defer q.lock.Unlock()
	for _, id := range ids {
		if !lo.Contains(q.ids, id) {
			q.ids = append(q.ids, id)
		}
	}
	return nil
}

func (q *dummyFulltextQueue) ListIdsFromFullTextQueue(limit int) ([]string, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if limit > len(q.ids) {
		limit = len(q.ids)
	}
	return q.ids[:limit], nil
}

func NewStoreFixture(t testing.TB) *StoreFixture {
	walletService := mock_wallet.NewMockWallet(t)
	walletService.EXPECT().Name().Return(wallet.CName).Maybe()
	walletService.EXPECT().RepoPath().Return(t.TempDir())

	provider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	fullText := ftsearch.TantivyNew()
	testApp := &app.App{}

	testApp.Register(walletService)
	err = fullText.Init(testApp)
	require.NoError(t, err)
	err = fullText.Run(context.Background())
	require.NoError(t, err)

	s := New(context.Background(), "test", Deps{
		Db:            provider.GetCommonDb(),
		Fts:           fullText,
		SourceService: &detailsFromId{},
		SubManager:    &SubscriptionManager{},
		FulltextQueue: &dummyFulltextQueue{},
	})
	return &StoreFixture{
		dsObjectStore: s.(*dsObjectStore),
	}
}

type TestObject map[domain.RelationKey]domain.Value

func (o TestObject) Id() string {
	return o[bundle.RelationKeyId].String()
}

func (o TestObject) Details() *domain.Details {
	return makeDetails(o)
}

func generateObjectWithRandomID() TestObject {
	id := fmt.Sprintf("%d", rand.Int())
	return TestObject{
		bundle.RelationKeyId:   domain.String(id),
		bundle.RelationKeyName: domain.String("name" + id),
	}
}

func makeObjectWithName(id string, name string) TestObject {
	return TestObject{
		bundle.RelationKeyId:      domain.String(id),
		bundle.RelationKeyName:    domain.String(name),
		bundle.RelationKeySpaceId: domain.String(spaceName),
	}
}

func makeObjectWithNameAndDescription(id string, name string, description string) TestObject {
	return TestObject{
		bundle.RelationKeyId:          domain.String(id),
		bundle.RelationKeyName:        domain.String(name),
		bundle.RelationKeyDescription: domain.String(description),
		bundle.RelationKeySpaceId:     domain.String(spaceName),
	}
}

func makeDetails(fields TestObject) *domain.Details {
	return domain.NewDetailsFromMap(fields)
}

func (fx *StoreFixture) AddObjects(t testing.TB, objects []TestObject) {
	for _, obj := range objects {
		id := obj[bundle.RelationKeyId].String()
		require.NotEmpty(t, id)
		err := fx.UpdateObjectDetails(context.Background(), id, makeDetails(obj))
		require.NoError(t, err)
	}
}
