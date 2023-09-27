package profiler

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("profiler")

type Service interface {
	app.ComponentRunnable
}

const (
	highMemoryUsageThreshold = 5 * 1024 * 1024 * 1024 // 5 Gb
	maxProfiles              = 3
	growthFactor             = 1.5
)

type service struct {
	closeCh chan struct{}

	timesHighMemoryUsageDetected int
	previousHighMemoryDetected   uint64
}

func New() Service {
	return &service{
		closeCh: make(chan struct{}),
	}
}

func (s *service) Init(a *app.App) (err error) {
	return nil
}

func (s *service) Name() (name string) {
	return "profiler"
}

func (s *service) Run(ctx context.Context) (err error) {
	go s.run()

	return nil
}

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

		log.With("sysMemory", s.previousHighMemoryDetected, "profile", base64.StdEncoding.EncodeToString(buf.Bytes())).Error("high memory usage detected, logging memory profile")
		fmt.Println(base64.StdEncoding.EncodeToString(buf.Bytes()))
		s.timesHighMemoryUsageDetected++

		if s.timesHighMemoryUsageDetected >= maxProfiles {
			return true, nil
		}
	}

	return false, nil
}

func (s *service) Close(ctx context.Context) (err error) {
	close(s.closeCh)
	return nil
}
