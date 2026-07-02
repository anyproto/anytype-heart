package spacev2

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

// techSpaceLoadDeadline bounds the first tech-space load attempt for existing
// accounts; hitting it triggers the offline old-account fallbacks.
const techSpaceLoadDeadline = 15 * time.Second

// techSpaceResolution abstracts the operations the tech-space resolution
// decision tree needs, so the tree itself is a pure, unit-testable function.
// The real implementations live on the service.
type techSpaceResolution struct {
	newAccount bool
	// create derives the tech space and runs it with create semantics.
	create func(ctx context.Context) (*clientspace.TechSpace, error)
	// load gets the existing tech space from storage/network and runs it.
	load func(ctx context.Context) (*clientspace.TechSpace, error)
	// personalCoreReachable probes that the personal space can be produced
	// (locally or from the network) — the precondition for the old-account
	// tech-space creation.
	personalCoreReachable func(ctx context.Context) error
	// personalStorageExists probes local storage only (no network).
	personalStorageExists func(ctx context.Context) (bool, error)
	// loadDeadline overrides techSpaceLoadDeadline in tests.
	loadDeadline time.Duration
}

// resolveTechSpace decides how this account gets its tech space:
//
//	new account                     → create
//	existing, load ok               → load
//	load timed out, personal local  → old account restored offline: create
//	load timed out, nothing local   → retry load without deadline (must get an
//	                                  authoritative answer before giving up)
//	nodes report no tech space      → old account: create
//
// oldAccount reports that the tech space was created for a pre-tech-space
// account, whose personal space then needs a SpaceView created for it.
// This is v1's initAccount fallback logic isolated into one explicit step.
func resolveTechSpace(ctx context.Context, r techSpaceResolution) (ts *clientspace.TechSpace, oldAccount bool, err error) {
	if r.newAccount {
		ts, err = r.create(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("create tech space: %w", err)
		}
		return ts, false, nil
	}
	deadline := r.loadDeadline
	if deadline <= 0 {
		deadline = techSpaceLoadDeadline
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, deadline)
	ts, err = r.load(timeoutCtx)
	cancel()
	if errors.Is(err, context.DeadlineExceeded) {
		personalExists, checkErr := r.personalStorageExists(ctx)
		if checkErr != nil {
			return nil, false, fmt.Errorf("check personal space storage: %w", checkErr)
		}
		if personalExists {
			// An old account restored locally with no connection: no tech
			// space will ever arrive, so create it from the local identity.
			return createTechSpaceForOldAccount(ctx, r)
		}
		ts, err = r.load(ctx)
	}
	if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
		// The nodes answered: this account has no tech space (pre-tech-space
		// account) — creating it is the only option.
		return createTechSpaceForOldAccount(ctx, r)
	}
	if err != nil {
		return nil, false, fmt.Errorf("load tech space: %w", err)
	}
	return ts, false, nil
}

func createTechSpaceForOldAccount(ctx context.Context, r techSpaceResolution) (*clientspace.TechSpace, bool, error) {
	// Without a personal space there is nothing to restore — creating a tech
	// space would only mask the broken account.
	if err := r.personalCoreReachable(ctx); err != nil {
		return nil, false, fmt.Errorf("get personal space: %w", err)
	}
	ts, err := r.create(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create tech space for old account: %w", err)
	}
	return ts, true, nil
}

// createTechSpace derives the tech space and runs it with create semantics.
func (s *service) createTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	techCore, err := s.spaceCore.Derive(ctx, spacedomain.SpaceTypeTech)
	if err != nil {
		return nil, fmt.Errorf("derive tech core space: %w", err)
	}
	return s.newTechSpace(techCore, true)
}

// loadTechSpace loads the existing tech space and reindexes it.
func (s *service) loadTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	techCore, err := s.spaceCore.Get(ctx, s.techSpaceId)
	if err != nil {
		return nil, fmt.Errorf("get tech core space: %w", err)
	}
	ts, err := s.newTechSpace(techCore, false)
	if err != nil {
		return nil, err
	}
	if err = s.indexer.ReindexSpace(ts); err != nil {
		return nil, fmt.Errorf("reindex tech space: %w", err)
	}
	return ts, nil
}

func (s *service) newTechSpace(techCore *spacecore.AnySpace, create bool) (*clientspace.TechSpace, error) {
	ts, err := clientspace.NewTechSpace(clientspace.TechSpaceDeps{
		CommonSpace:      techCore,
		ObjectFactory:    s.objectFactory,
		AccountService:   s.accountService,
		PersonalSpaceId:  s.personalSpaceId,
		Indexer:          s.indexer,
		Installer:        s.installer,
		TechSpace:        techspace.New(),
		KeyValueObserver: techCore.KeyValueObserver(),
	})
	if err != nil {
		return nil, fmt.Errorf("build tech space: %w", err)
	}
	if err = ts.Run(techCore, ts.Cache, create); err != nil {
		return nil, fmt.Errorf("run tech space: %w", err)
	}
	return ts, nil
}

// ensurePersonalSpaceView creates the personal space's SpaceView when it is
// missing — needed after the tech space was created for an old account (whose
// personal space predates SpaceViews). The personal space itself already
// exists; only the view is created here, with status Unknown so the watcher
// picks it up and loads it.
func (s *service) ensurePersonalSpaceView(ctx context.Context) error {
	info := spaceinfo.NewSpacePersistentInfo(s.personalSpaceId)
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
	err := s.techSpace.SpaceViewCreate(ctx, s.personalSpaceId, false, info, nil)
	if err != nil && !errors.Is(err, techspace.ErrSpaceViewExists) {
		return fmt.Errorf("create personal space view: %w", err)
	}
	return nil
}

// marketplaceSpace is the static entry serving the deprecated marketplace
// virtual space (bundled types/relations). It is not a controller: nothing
// syncs, loads, or offloads.
type marketplaceSpace struct {
	vs      clientspace.Space
	indexer dependencies.SpaceIndexer

	reindexOnce sync.Once
	reindexErr  error
}

// Get returns the virtual space, reindexing it on first use. Unlike v1, a
// failed reindex keeps reporting its error instead of silently succeeding on
// the second call.
func (m *marketplaceSpace) Get() (clientspace.Space, error) {
	m.reindexOnce.Do(func() {
		m.reindexErr = m.indexer.ReindexMarketplaceSpace(m.vs)
	})
	if m.reindexErr != nil {
		return nil, fmt.Errorf("reindex marketplace space: %w", m.reindexErr)
	}
	return m.vs, nil
}

func (s *service) initMarketplace() error {
	s.marketplace = &marketplaceSpace{
		vs: clientspace.NewVirtualSpace(addr.AnytypeMarketplaceWorkspace, clientspace.VirtualSpaceDeps{
			ObjectFactory:   s.objectFactory,
			AccountService:  s.accountService,
			PersonalSpaceId: s.personalSpaceId,
			Indexer:         s.indexer,
			Installer:       s.installer,
			TypePrefix:      addr.BundledObjectTypeURLPrefix,
			RelationPrefix:  addr.BundledRelationURLPrefix,
		}),
		indexer: s.indexer,
	}
	if err := s.virtualSpaceService.RegisterVirtualSpace(addr.AnytypeMarketplaceWorkspace); err != nil {
		return fmt.Errorf("register marketplace virtual space: %w", err)
	}
	return nil
}
