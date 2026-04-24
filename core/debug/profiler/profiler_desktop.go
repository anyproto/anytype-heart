//go:build !gomobile

package profiler

import (
	"runtime"
	"time"

	"github.com/anyproto/anytype-heart/core/debug/debugreporter"
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
			if s.detect() {
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

// detect checks the memory-growth heuristic and, when it trips, reports a
// heap snapshot via the Reporter path (artifact + event in one call).
// Returns true after maxProfiles triggers so the background goroutine exits
// rather than filling the profiles directory indefinitely.
func (s *service) detect() (stop bool) {
	if !s.isMemoryGrowing() {
		return false
	}

	s.Report(reasonMemoryGrowth, map[string]any{
		"sysMemory": s.previousHighMemoryDetected,
	}, debugreporter.Capture{Kind: debugreporter.KindHeap})
	s.timesHighMemoryUsageDetected++
	log.Warnw("memory growth detected", "sysMemory", s.previousHighMemoryDetected)

	return s.timesHighMemoryUsageDetected >= maxProfiles
}
