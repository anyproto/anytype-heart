package objectstore

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

type StoreFixture struct {
	*dsObjectStore
	FullText ftsearch.FTSearch
}

// func (fx *StoreFixture) TechSpaceId() string {
// 	return fx.techSpaceIdProvider.TechSpaceId()
// }

type virtualDetailsHandler interface {
	AddVirtualDetails(id string, det *domain.Details)
}

type stubDetailsFromId struct {
	details map[string]*domain.Details
}

func (d *stubDetailsFromId) Name() string {
	return "stubDetailsFromId"
}

func (d *stubDetailsFromId) Init(a *app.App) error {
	return nil
}

func (d *stubDetailsFromId) DetailsFromIdBasedSource(id domain.FullID) (*domain.Details, error) {
	if det, found := d.details[id.ObjectID]; found {
		return det, nil
	}
	return nil, fmt.Errorf("not found")
}

func (d *stubDetailsFromId) AddVirtualDetails(id string, det *domain.Details) {
	if d.details == nil {
		d.details = map[string]*domain.Details{}
	}
	d.details[id] = det
}

const TestTechSpaceId = "test-tech-space"

type stubTechSpaceIdProvider struct{}

func (s *stubTechSpaceIdProvider) TechSpaceId() string {
	return TestTechSpaceId
}

func (s *stubTechSpaceIdProvider) Name() string {
	return "stubTechSpaceIdProvider"
}

func (s *stubTechSpaceIdProvider) Init(a *app.App) error {
	return nil
}

type walletStub struct {
	wallet.Wallet
	tempDir string
}

func newWalletStub(t testing.TB) wallet.Wallet {
	return &walletStub{
		tempDir: t.TempDir(),
	}
}

func (w *walletStub) FtsPrimaryLang() string {
	return ""
}

func (w *walletStub) RepoPath() string {
	return w.tempDir
}

func (w *walletStub) Name() string { return wallet.CName }

type spaceIdsListerStub struct{ ids []string }

func (s *spaceIdsListerStub) AllSpaceIds() (ids []string, err error) { return s.ids, nil }
func (s *spaceIdsListerStub) Name() string                          { return "spaceIdsListerStub" }
func (s *spaceIdsListerStub) Init(a *app.App) error                 { return nil }

func NewStoreFixture(t testing.TB) *StoreFixture {
	return newStoreFixture(t)
}

func NewStoreFixtureWithSpaceIds(t testing.TB, ids []string) *StoreFixture {
	return newStoreFixture(t, &spaceIdsListerStub{ids: ids})
}

func newStoreFixture(t testing.TB, extra ...app.Component) *StoreFixture {
	ctx := context.Background()

	fullText := ftsearch.TantivyNew()
	testApp := &app.App{}

	testApp.Register(newWalletStub(t))
	err := fullText.Init(testApp)
	require.NoError(t, err)

	provider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	testApp.Register(provider)
	testApp.Register(fullText)
	testApp.Register(&stubDetailsFromId{})
	testApp.Register(&stubTechSpaceIdProvider{})
	for _, c := range extra {
		testApp.Register(c)
	}

	err = fullText.Init(testApp)
	require.NoError(t, err)
	err = fullText.Run(context.Background())
	require.NoError(t, err)

	ds := New()

	t.Cleanup(func() {
		err = fullText.Close(context.Background())
		if err != nil {
			t.Fatal("FOTAL:", err)
		}
		_ = ds.Close(context.Background())
	})

	err = ds.Init(testApp)
	require.NoError(t, err)

	err = ds.Run(ctx)
	require.NoError(t, err)

	return &StoreFixture{
		dsObjectStore: ds.(*dsObjectStore),
		FullText:      fullText,
	}
}

func (fx *StoreFixture) Init(a *app.App) (err error) {
	return nil
}

// Run is a no-op: newStoreFixture already Init+Run the underlying
// dsObjectStore. Registering the fixture into a second app must not invoke
// dsObjectStore.Run a second time (component Run is a once-per-instance
// invariant; a second close of loadedCh would panic).
func (fx *StoreFixture) Run(ctx context.Context) error {
	return nil
}

type TestObject = spaceindex.TestObject

func (fx *StoreFixture) AddObjects(t testing.TB, spaceId string, objects []spaceindex.TestObject) {
	store := fx.SpaceIndex(spaceId)
	for _, obj := range objects {
		id := obj[bundle.RelationKeyId].String()
		require.NotEmpty(t, id)
		err := store.UpdateObjectDetails(context.Background(), id, makeDetails(obj))
		require.NoError(t, err)
	}
}

func makeDetails(fields spaceindex.TestObject) *domain.Details {
	return domain.NewDetailsFromMap(fields)
}

func (fx *StoreFixture) AddVirtualDetails(id string, details *domain.Details) {
	if handler := fx.sourceService.(virtualDetailsHandler); handler != nil {
		handler.AddVirtualDetails(id, details)
	}
}

func (fx *StoreFixture) GetDetails(spaceId, objectId string) (*domain.Details, error) {
	return fx.SpaceIndex(spaceId).GetDetails(objectId)
}
