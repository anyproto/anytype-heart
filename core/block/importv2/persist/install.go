package persist

import (
	"context"
	"fmt"
	"sync"
)

// BundledInstaller installs bundled relations/types by their bundled URLs.
// The wiring layer adapts objectcreator.Service.InstallBundledObjects (which
// needs the concrete clientspace.Space) onto this seam.
type BundledInstaller interface {
	InstallBundledObjects(ctx context.Context, ids []string) error
}

// InstallCoordinator serializes and dedups bundled-object installation across
// persist workers: each bundled id is installed at most once per run, and
// concurrent workers never race the installer (v1 ran up to 10 concurrent
// installs of the same ids and leaned on the installer's internal dedup).
type InstallCoordinator struct {
	installer BundledInstaller

	mu        sync.Mutex
	installed map[string]struct{}
}

func NewInstallCoordinator(installer BundledInstaller) *InstallCoordinator {
	return &InstallCoordinator{
		installer: installer,
		installed: map[string]struct{}{},
	}
}

// Ensure installs the not-yet-installed subset of ids. Successful ids are
// remembered; failed ones will be retried by the next caller.
func (c *InstallCoordinator) Ensure(ctx context.Context, ids []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	missing := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, done := c.installed[id]; !done {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := c.installer.InstallBundledObjects(ctx, missing); err != nil {
		return fmt.Errorf("install bundled objects: %w", err)
	}
	for _, id := range missing {
		c.installed[id] = struct{}{}
	}
	return nil
}
