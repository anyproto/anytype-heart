package spacev2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
)

type loadResult struct {
	ts  *clientspace.TechSpace
	err error
}

type fakeTechProvider struct {
	createResult *clientspace.TechSpace
	createErr    error
	loadResults  []loadResult

	createCalls int
	loadCalls   int
}

func (f *fakeTechProvider) Create(ctx context.Context) (*clientspace.TechSpace, error) {
	f.createCalls++
	return f.createResult, f.createErr
}

func (f *fakeTechProvider) Load(ctx context.Context) (*clientspace.TechSpace, error) {
	require.Less(&testing.T{}, f.loadCalls, len(f.loadResults))
	res := f.loadResults[f.loadCalls]
	f.loadCalls++
	return res.ts, res.err
}

type resolveFixture struct {
	*service
	provider  *fakeTechProvider
	spaceCore *mock_spacecore.MockSpaceCoreService
	techMock  *mock_techspace.MockTechSpace
}

func newResolveFixture(t *testing.T, newAccount bool, provider *fakeTechProvider) *resolveFixture {
	fx := &resolveFixture{
		provider:  provider,
		spaceCore: mock_spacecore.NewMockSpaceCoreService(t),
		techMock:  mock_techspace.NewMockTechSpace(t),
	}
	fx.service = &service{
		newAccount:      newAccount,
		personalSpaceId: testPersonalSpaceId,
		techSpaceId:     testTechSpaceId,
		spaceCore:       fx.spaceCore,
		techProvider:    provider,
		techSpaceReady:  make(chan struct{}),
	}
	return fx
}

func (fx *resolveFixture) techSpaceFor() *clientspace.TechSpace {
	return &clientspace.TechSpace{TechSpace: fx.techMock}
}

func (fx *resolveFixture) assertReady(t *testing.T) {
	readyCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ts, err := fx.getTechSpace(readyCtx)
	require.NoError(t, err)
	assert.NotNil(t, ts)
}

func (fx *resolveFixture) assertNotReady(t *testing.T) {
	readyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := fx.getTechSpace(readyCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestService_ResolveTechSpace(t *testing.T) {
	t.Run("new account creates the tech space", func(t *testing.T) {
		// given
		provider := &fakeTechProvider{}
		fx := newResolveFixture(t, true, provider)
		provider.createResult = fx.techSpaceFor()

		// when
		err := fx.resolveTechSpace(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, provider.createCalls)
		assert.Equal(t, 0, provider.loadCalls)
		fx.assertReady(t)
	})

	t.Run("existing account loads the tech space", func(t *testing.T) {
		// given
		provider := &fakeTechProvider{}
		fx := newResolveFixture(t, false, provider)
		provider.loadResults = []loadResult{{ts: fx.techSpaceFor()}}

		// when
		err := fx.resolveTechSpace(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, provider.loadCalls)
		assert.Equal(t, 0, provider.createCalls)
		fx.assertReady(t)
	})

	t.Run("deadline without local personal space retries load", func(t *testing.T) {
		// given: no internet, then internet appeared (v1 case)
		provider := &fakeTechProvider{}
		fx := newResolveFixture(t, false, provider)
		provider.loadResults = []loadResult{
			{err: context.DeadlineExceeded},
			{ts: fx.techSpaceFor()},
		}
		fx.spaceCore.EXPECT().StorageExistsLocally(mock.Anything, testPersonalSpaceId).Return(false, nil)

		// when
		err := fx.resolveTechSpace(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, provider.loadCalls)
		assert.Equal(t, 0, provider.createCalls)
		fx.assertReady(t)
	})

	t.Run("deadline with local personal space creates tech space and personal view", func(t *testing.T) {
		// given: old account restored offline (v1 case)
		provider := &fakeTechProvider{}
		fx := newResolveFixture(t, false, provider)
		provider.loadResults = []loadResult{{err: context.DeadlineExceeded}}
		provider.createResult = fx.techSpaceFor()
		fx.spaceCore.EXPECT().StorageExistsLocally(mock.Anything, testPersonalSpaceId).Return(true, nil)
		fx.spaceCore.EXPECT().Get(mock.Anything, testPersonalSpaceId).Return(nil, nil)
		fx.techMock.EXPECT().SpaceViewExists(mock.Anything, testPersonalSpaceId).Return(false, nil)
		fx.techMock.EXPECT().SpaceViewCreate(mock.Anything, testPersonalSpaceId, true, mock.Anything, mock.Anything).Return(nil)

		// when
		err := fx.resolveTechSpace(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, provider.loadCalls)
		assert.Equal(t, 1, provider.createCalls)
		fx.assertReady(t)
	})

	t.Run("space missing on nodes creates tech space, existing view untouched", func(t *testing.T) {
		// given: very old account without tech space (v1 case)
		provider := &fakeTechProvider{}
		fx := newResolveFixture(t, false, provider)
		provider.loadResults = []loadResult{{err: spacesyncproto.ErrSpaceMissing}}
		provider.createResult = fx.techSpaceFor()
		fx.spaceCore.EXPECT().Get(mock.Anything, testPersonalSpaceId).Return(nil, nil)
		fx.techMock.EXPECT().SpaceViewExists(mock.Anything, testPersonalSpaceId).Return(true, nil)

		// when
		err := fx.resolveTechSpace(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, provider.createCalls)
		fx.assertReady(t)
	})

	t.Run("other load error fails and leaves tech space not ready", func(t *testing.T) {
		// given
		provider := &fakeTechProvider{}
		fx := newResolveFixture(t, false, provider)
		wantErr := errors.New("storage exploded")
		provider.loadResults = []loadResult{{err: wantErr}}

		// when
		err := fx.resolveTechSpace(ctx)

		// then
		require.ErrorIs(t, err, wantErr)
		assert.Equal(t, 0, provider.createCalls)
		fx.assertNotReady(t)
	})
}
