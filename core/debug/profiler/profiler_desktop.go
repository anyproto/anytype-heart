//go:build !gomobile

package profiler

import (
	"fmt"
	"runtime"
	"time"

	"github.com/anyproto/anytype-heart/core/debug/debugsnapshot"
	"github.com/anyproto/anytype-heart/pkg/lib/initialparams"
)

const (
	highMemoryUsageThreshold = 1024 * 1024 * 1024 // 1 GiB system memory
	maxProfiles              = 3
	growthFactor             = 1.5
	reasonMemoryGrowth       = "MEMORY_GROWTH"
)

func (s *service) run() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stop, err := s.detect()
			if err != nil {
				log.Errorf("memory-growth detector error: %s", err)
			}
			if stop {
				return
			}
		case <-s.closeCh:
			return
		}
	}
}

// isMemoryGrowing samples runtime.MemStats.Sys. It trips on the first
// crossing of highMemoryUsageThreshold and again whenever Sys grows by
// growthFactor (1.5x) past the previous trip value.
func (s *service) isMemoryGrowing() bool {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	if s.previousHighMemoryDetected == 0 && stats.Sys > highMemoryUsageThreshold {
		s.previousHighMemoryDetected = stats.Sys
		return true
	}

	if s.previousHighMemoryDetected > 0 && stats.Sys > uint64(float64(s.previousHighMemoryDetected)*growthFactor) {
		s.previousHighMemoryDetected = stats.Sys
		return true
	}

	return false
}

// detect writes a debug snapshot into profilesDir when memory growth is
// detected. Returns stop=true after maxProfiles triggers so the background
// goroutine exits instead of filling the profiles directory endlessly.
func (s *service) detect() (stop bool, err error) {
	if !s.isMemoryGrowing() {
		return false, nil
	}

	paths := initialparams.Get().Paths
	if paths.ProfilesDir == "" {
		// Not yet configured (InitialSetParameters hasn't been called, or
		// workdir is empty). Nothing we can write — skip this tick.
		return false, nil
	}

	reasonDesc := fmt.Sprintf("sysMemory=%d", s.previousHighMemoryDetected)
	path, err := debugsnapshot.Save(paths.ProfilesDir, reasonMemoryGrowth, reasonDesc, debugsnapshot.Meta{
		RootPath: paths.Workdir,
	})
	if err != nil {
		return false, fmt.Errorf("save memory-growth snapshot: %w", err)
	}

	log.With("sysMemory", s.previousHighMemoryDetected, "snapshot", path).Warn("memory growth detected, snapshot saved")
	s.timesHighMemoryUsageDetected++

	return s.timesHighMemoryUsageDetected >= maxProfiles, nil
}
