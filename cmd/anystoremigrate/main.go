// Command anystoremigrate migrates a copy of an account's v1 object store directory
// (objectstore/objects.db, objectstore/<spaceId>/{objects.db,crdt.db}) to any-store v2 in place,
// with the migrator the provider runs at startup, and reports how it went. It is meant for copies:
// point it at a clone of the store, never at a live account.
//
//	go run ./cmd/anystoremigrate -dir /path/to/objectstore-copy -limit 1 -verify
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoremigrate"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/util/freespace"
)

type summary struct {
	Dir             string        `json:"dir"`
	Limit           int           `json:"limit"`
	Databases       int           `json:"databases"`
	Total           time.Duration `json:"total"`
	Largest         dbSummary     `json:"largest"`
	Slowest         []dbSummary   `json:"slowest"`
	DurationsP50    time.Duration `json:"durationP50"`
	DurationsP90    time.Duration `json:"durationP90"`
	DurationsMax    time.Duration `json:"durationMax"`
	SrcBytes        int64         `json:"srcBytes"`
	DstBytes        int64         `json:"dstBytes"`
	Documents       int           `json:"documents"`
	ChatCollections int           `json:"chatCollections"`
	StubsDropped    int           `json:"stubsDropped"`
	PeakWalBytes    int64         `json:"peakWalBytes"`
	PeakExtraDisk   uint64        `json:"peakExtraDiskBytes"`
	PeakHeapBytes   uint64        `json:"peakHeapBytes"`
	Verify          *verifyResult `json:"verify,omitempty"`
}

type dbSummary struct {
	Path     string        `json:"path"`
	SrcBytes int64         `json:"srcBytes"`
	DstBytes int64         `json:"dstBytes"`
	Duration time.Duration `json:"duration"`
	PeakWal  int64         `json:"peakWal"`
}

func main() {
	var (
		dir      = flag.String("dir", "", "object store directory to migrate in place (a copy)")
		limit    = flag.Int("limit", 1, "databases migrating at once")
		removeV1 = flag.Bool("remove-v1", false, "remove each v1 file once its verified v2 file is in place")
		doVerify = flag.Bool("verify", false, "compare every migrated database with its kept v1 file, document by document")
		out      = flag.String("out", "", "write the summary as JSON to this file")
		cpu      = flag.String("cpuprofile", "", "write a CPU profile of the migration to this file")
	)
	flag.Parse()
	if *cpu != "" {
		f, err := os.Create(*cpu)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err = pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}
	if *doVerify && *removeV1 {
		fmt.Fprintln(os.Stderr, "-verify needs the v1 files: drop -remove-v1")
		os.Exit(2)
	}
	sum, err := run(context.Background(), *dir, *limit, *removeV1, *doVerify)
	pprof.StopCPUProfile()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(sum, "", "  ")
	fmt.Println(string(data))
	if *out != "" {
		if err = os.WriteFile(*out, data, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func run(ctx context.Context, dir string, limit int, removeV1, doVerify bool) (*summary, error) {
	paths, err := anystoreprovider.DatabasePaths(dir)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	opts := anystoreprovider.MigrationOptions(nil)
	opts.Concurrency = limit
	opts.RemoveV1AfterVerify = removeV1
	m := anystoremigrate.New(opts)
	if err = m.CheckSpace(paths); err != nil {
		return nil, fmt.Errorf("check space: %w", err)
	}

	startFree, err := freespace.GetFreeDiskSpace(dir)
	if err != nil {
		return nil, fmt.Errorf("free space: %w", err)
	}
	var (
		minFree  atomic.Uint64
		peakHeap atomic.Uint64
		done     = make(chan struct{})
		sampled  sync.WaitGroup
	)
	minFree.Store(startFree)
	sampled.Add(1)
	go func() {
		defer sampled.Done()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		var ms runtime.MemStats
		for {
			if free, err := freespace.GetFreeDiskSpace(dir); err == nil && free < minFree.Load() {
				minFree.Store(free)
			}
			runtime.ReadMemStats(&ms)
			if ms.HeapInuse > peakHeap.Load() {
				peakHeap.Store(ms.HeapInuse)
			}
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()

	start := time.Now()
	reports := make([]anystoremigrate.Report, len(paths))
	errs := make([]error, len(paths))
	var wg sync.WaitGroup
	for i, p := range paths {
		isV1, err := anystoremigrate.IsV1File(p)
		if err != nil {
			return nil, fmt.Errorf("check %s: %w", p, err)
		}
		if !isV1 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			reports[i], errs[i] = m.MigrateFile(ctx, p)
		}()
	}
	wg.Wait()
	total := time.Since(start)
	close(done)
	sampled.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("migrate %s: %w", paths[i], err)
		}
	}

	sum := &summary{Dir: dir, Limit: limit, Total: total, PeakExtraDisk: startFree - minFree.Load(), PeakHeapBytes: peakHeap.Load()}
	var all []dbSummary
	for _, r := range reports {
		if r.Path == "" {
			continue
		}
		rel, _ := filepath.Rel(dir, r.Path)
		db := dbSummary{Path: rel, SrcBytes: r.SrcBytes, DstBytes: r.DstBytes, Duration: r.Duration, PeakWal: r.PeakWalBytes}
		all = append(all, db)
		sum.Databases++
		sum.SrcBytes += r.SrcBytes
		sum.DstBytes += r.DstBytes
		sum.Documents += r.Documents
		sum.ChatCollections += r.ChatCollections
		sum.StubsDropped += r.StubsDropped
		sum.PeakWalBytes = max(sum.PeakWalBytes, r.PeakWalBytes)
		if r.SrcBytes > sum.Largest.SrcBytes {
			sum.Largest = db
		}
	}
	slices.SortFunc(all, func(a, b dbSummary) int { return int(b.Duration - a.Duration) })
	sum.Slowest = all[:min(5, len(all))]
	if len(all) > 0 {
		durations := make([]time.Duration, len(all))
		for i, db := range all {
			durations[i] = db.Duration
		}
		slices.Sort(durations)
		sum.DurationsP50 = durations[len(durations)/2]
		sum.DurationsP90 = durations[len(durations)*9/10]
		sum.DurationsMax = durations[len(durations)-1]
	}
	if doVerify {
		v, err := verifyAll(ctx, reports)
		if err != nil {
			return nil, fmt.Errorf("verify: %w", err)
		}
		sum.Verify = v
	}
	return sum, nil
}
