package spacev2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

var loadTechSpaceDeadline = 15 * time.Second

// resolveTechSpace makes s.techSpace usable, isolating the old-account /
// offline-restore fallbacks behind one explicit step (docs/SpaceController.md
// §11.7; decision table in the M2 plan). Closes techSpaceReady on success only:
// on failure Get(techSpaceId) must keep blocking rather than return nil (§9).
func (s *service) resolveTechSpace(ctx context.Context) (err error) {
	if s.newAccount {
		return s.createTechSpace(ctx)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, loadTechSpaceDeadline)
	err = s.loadTechSpace(timeoutCtx)
	cancel()
	if errors.Is(err, context.DeadlineExceeded) {
		// Offline restore of an account this device has never synced: the
		// nodes are unreachable, so decide from local state.
		var personalExists bool
		personalExists, err = s.spaceCore.StorageExistsLocally(ctx, s.personalSpaceId)
		if err != nil {
			return fmt.Errorf("check personal space storage: %w", err)
		}
		if !personalExists {
			// Nothing local: only the nodes can tell whether a tech space
			// exists, so retry without the bootstrap deadline.
			err = s.loadTechSpace(ctx)
		} else {
			return s.createTechSpaceForOldAccount(ctx)
		}
	}
	if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
		// The nodes answered: no tech space anywhere — the account predates
		// tech spaces. Creating one is the only option.
		return s.createTechSpaceForOldAccount(ctx)
	}
	if err != nil {
		return fmt.Errorf("resolve tech space: %w", err)
	}
	return nil
}

func (s *service) createTechSpace(ctx context.Context) (err error) {
	if s.techSpace, err = s.techProvider.Create(ctx); err != nil {
		return fmt.Errorf("create tech space: %w", err)
	}
	close(s.techSpaceReady)
	return nil
}

func (s *service) loadTechSpace(ctx context.Context) error {
	// Sentinel errors (DeadlineExceeded, ErrSpaceMissing) are inspected by
	// resolveTechSpace; the provider already wraps with operation context.
	ts, err := s.techProvider.Load(ctx)
	if err != nil {
		return err
	}
	s.techSpace = ts
	close(s.techSpaceReady)
	return nil
}

// createTechSpaceForOldAccount handles accounts that predate tech spaces: the
// personal space exists (locally or per the nodes' ErrSpaceMissing answer) but
// no tech space does. Create the tech space and ensure the personal space's
// SpaceView exists; the watcher then builds the personal controller through
// the regular unidirectional path (replaces v1's SkipCheckSpaceViewKey hack).
func (s *service) createTechSpaceForOldAccount(ctx context.Context) error {
	if _, err := s.spaceCore.Get(ctx, s.personalSpaceId); err != nil {
		return fmt.Errorf("get personal space: %w", err)
	}
	if err := s.createTechSpace(ctx); err != nil {
		return err
	}
	exists, err := s.techSpace.SpaceViewExists(ctx, s.personalSpaceId)
	if err != nil {
		return fmt.Errorf("check personal space view: %w", err)
	}
	if !exists {
		info := spaceinfo.NewSpacePersistentInfo(s.personalSpaceId)
		info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
		if err = s.techSpace.SpaceViewCreate(ctx, s.personalSpaceId, true, info, nil); err != nil {
			return fmt.Errorf("create personal space view: %w", err)
		}
	}
	return nil
}

func (s *service) getTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	select {
	case <-s.techSpaceReady:
		return s.techSpace, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
