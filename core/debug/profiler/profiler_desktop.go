//go:build !gomobile

package profiler

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"runtime"
	"runtime/pprof"
	"time"
)

const (
	highMemoryUsageThreshold = 1024 * 1024 // 1 Gb
	maxProfiles              = 3
	growthFactor             = 1.5
)

func (s *service) run() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stop, err := s.detect()
			if stop {
				return
			}
			if err != nil {
				log.Errorf("high memory detector error: %s", err)
			}
		case <-s.closeCh:
			return
		}
	}
}

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

func (s *service) detect() (stop bool, err error) {
	if s.isMemoryGrowing() {
		buf := &bytes.Buffer{}
		gzipWriter := gzip.NewWriter(buf)
		err := pprof.WriteHeapProfile(gzipWriter)
		if err != nil {
			return stop, fmt.Errorf("write heap profile: %w", err)
		}
		gzipWriter.Close()

		// To extract profile from logged string use `base64 -d | gzip -d`
		log.With("sysMemory", s.previousHighMemoryDetected, "profile", base64.StdEncoding.EncodeToString(buf.Bytes())).Error("high memory usage detected, logging memory profile")
		s.timesHighMemoryUsageDetected++

		if s.timesHighMemoryUsageDetected >= maxProfiles {
			return true, nil
		}
	}

	return false, nil
}
