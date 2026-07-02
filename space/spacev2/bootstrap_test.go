package spacev2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

// resolutionFixture scripts the tech-space resolution seams. loadResults are
// consumed per load call; a fake deadline-load is simulated by returning
// context.DeadlineExceeded from the scripted result (the tree only inspects
// errors, never real time).
type resolutionFixture struct {
	loadResults   []error
	createErr     error
	reachableErr  error
	storageExists bool
	storageErr    error

	loads     int
	creates   int
	reachable int
}

func (f *resolutionFixture) resolution(newAccount bool) techSpaceResolution {
	ts := &clientspace.TechSpace{}
	return techSpaceResolution{
		newAccount:   newAccount,
		loadDeadline: 50 * time.Millisecond,
		create: func(ctx context.Context) (*clientspace.TechSpace, error) {
			f.creates++
			if f.createErr != nil {
				return nil, f.createErr
			}
			return ts, nil
		},
		load: func(ctx context.Context) (*clientspace.TechSpace, error) {
			f.loads++
			var err error
			if len(f.loadResults) > 0 {
				err = f.loadResults[0]
				f.loadResults = f.loadResults[1:]
			}
			if err != nil {
				return nil, err
			}
			return ts, nil
		},
		personalCoreReachable: func(ctx context.Context) error {
			f.reachable++
			return f.reachableErr
		},
		personalStorageExists: func(ctx context.Context) (bool, error) {
			return f.storageExists, f.storageErr
		},
	}
}

func TestResolveTechSpaceNewAccountCreates(t *testing.T) {
	fx := &resolutionFixture{}

	ts, err := resolveTechSpace(context.Background(), fx.resolution(true))

	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, 1, fx.creates)
	assert.Zero(t, fx.loads)
}

func TestResolveTechSpaceExistingLoads(t *testing.T) {
	fx := &resolutionFixture{}

	ts, err := resolveTechSpace(context.Background(), fx.resolution(false))

	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, 1, fx.loads)
	assert.Zero(t, fx.creates)
}

func TestResolveTechSpaceOfflineOldAccountCreates(t *testing.T) {
	// given: the network never answers, but the personal space is on disk —
	// an old account restored offline
	fx := &resolutionFixture{
		loadResults:   []error{context.DeadlineExceeded},
		storageExists: true,
	}

	ts, err := resolveTechSpace(context.Background(), fx.resolution(false))

	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, 1, fx.loads)
	assert.Equal(t, 1, fx.reachable, "must verify the personal space is producible before creating")
	assert.Equal(t, 1, fx.creates)
}

func TestResolveTechSpaceTimeoutRetriesWithoutDeadline(t *testing.T) {
	// given: timeout with nothing local — an authoritative answer is required
	fx := &resolutionFixture{
		loadResults: []error{context.DeadlineExceeded, nil},
	}

	ts, err := resolveTechSpace(context.Background(), fx.resolution(false))

	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, 2, fx.loads)
	assert.Zero(t, fx.creates)
}

func TestResolveTechSpaceRetryReportsMissingCreates(t *testing.T) {
	// given: retry gets the authoritative "no tech space" answer
	fx := &resolutionFixture{
		loadResults: []error{context.DeadlineExceeded, spacesyncproto.ErrSpaceMissing},
	}

	ts, err := resolveTechSpace(context.Background(), fx.resolution(false))

	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, 2, fx.loads)
	assert.Equal(t, 1, fx.creates)
}

func TestResolveTechSpaceMissingOnFirstLoadCreates(t *testing.T) {
	fx := &resolutionFixture{
		loadResults: []error{spacesyncproto.ErrSpaceMissing},
	}

	ts, err := resolveTechSpace(context.Background(), fx.resolution(false))

	require.NoError(t, err)
	require.NotNil(t, ts)
	assert.Equal(t, 1, fx.creates)
}

func TestResolveTechSpaceOtherLoadErrorFails(t *testing.T) {
	loadErr := errors.New("storage corrupted")
	fx := &resolutionFixture{loadResults: []error{loadErr}}

	_, err := resolveTechSpace(context.Background(), fx.resolution(false))

	require.ErrorIs(t, err, loadErr)
	assert.Zero(t, fx.creates)
}

func TestResolveTechSpaceOldAccountWithoutPersonalFails(t *testing.T) {
	// given: nodes report missing tech space AND the personal space is not
	// producible — a broken account must not get a fresh empty tech space
	reachErr := errors.New("personal space unreachable")
	fx := &resolutionFixture{
		loadResults:  []error{spacesyncproto.ErrSpaceMissing},
		reachableErr: reachErr,
	}

	_, err := resolveTechSpace(context.Background(), fx.resolution(false))

	require.ErrorIs(t, err, reachErr)
	assert.Zero(t, fx.creates)
}

func TestResolveTechSpaceStorageCheckErrorFails(t *testing.T) {
	storageErr := errors.New("storage check failed")
	fx := &resolutionFixture{
		loadResults: []error{context.DeadlineExceeded},
		storageErr:  storageErr,
	}

	_, err := resolveTechSpace(context.Background(), fx.resolution(false))

	require.ErrorIs(t, err, storageErr)
	assert.Zero(t, fx.creates)
}

// viewCreatingTechSpace extends the backend-test fake with SpaceViewCreate.
type viewCreatingTechSpace struct {
	techspace.TechSpace
	created   []string
	createErr error
}

func (t *viewCreatingTechSpace) SpaceViewCreate(ctx context.Context, spaceId string, force bool, info spaceinfo.SpacePersistentInfo, desc *spaceinfo.SpaceDescription) error {
	if t.createErr != nil {
		return t.createErr
	}
	t.created = append(t.created, spaceId)
	return nil
}

func TestEnsurePersonalSpaceExistingAccount(t *testing.T) {
	// given: an existing account (no derive/mark) whose view may be missing
	fakeTS := &viewCreatingTechSpace{}
	s := &service{personalSpaceId: "personal1"}
	s.techSpace = &clientspace.TechSpace{TechSpace: fakeTS}

	// when
	err := s.ensurePersonalSpace(context.Background())

	// then
	require.NoError(t, err)
	assert.Equal(t, []string{"personal1"}, fakeTS.created)
}

func TestEnsurePersonalSpaceViewExistsIsFine(t *testing.T) {
	// given: the view already exists
	fakeTS := &viewCreatingTechSpace{createErr: techspace.ErrSpaceViewExists}
	s := &service{personalSpaceId: "personal1"}
	s.techSpace = &clientspace.TechSpace{TechSpace: fakeTS}

	// when
	err := s.ensurePersonalSpace(context.Background())

	// then
	require.NoError(t, err)
}

func TestMarketplaceGetReindexesOnceAndKeepsError(t *testing.T) {
	// given
	indexer := &marketplaceIndexer{err: errors.New("reindex failed")}
	m := &marketplaceSpace{vs: &fakeSpace{}, indexer: indexer}

	// when: first Get fails
	_, err := m.Get()

	// then: the error is kept, not swallowed on the next call (v1 bug fixed)
	require.Error(t, err)
	_, err = m.Get()
	require.Error(t, err)
	assert.Equal(t, 1, indexer.calls)

	// and: a fresh marketplace with a working indexer reindexes exactly once
	indexer2 := &marketplaceIndexer{}
	m2 := &marketplaceSpace{vs: &fakeSpace{}, indexer: indexer2}
	sp, err := m2.Get()
	require.NoError(t, err)
	require.NotNil(t, sp)
	_, err = m2.Get()
	require.NoError(t, err)
	assert.Equal(t, 1, indexer2.calls)
}

type marketplaceIndexer struct {
	dependencies.SpaceIndexer
	calls int
	err   error
}

func (m *marketplaceIndexer) ReindexMarketplaceSpace(space clientspace.Space) error {
	m.calls++
	return m.err
}
