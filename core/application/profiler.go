package application

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/klauspost/compress/flate"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
	exptrace "golang.org/x/exp/trace"

	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/initialparams"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/debug"
	"github.com/anyproto/anytype-heart/util/vcs"
)

var ErrNoFolder = fmt.Errorf("no folder provided")

func getCPUModel() string {
	infos, err := cpu.Info()
	if err != nil || len(infos) == 0 {
		return ""
	}
	return infos[0].ModelName
}

func getSystemMemory() *systemMemory {
	v, err := mem.VirtualMemory()
	if err != nil {
		return nil
	}
	return &systemMemory{
		TotalMB:     v.Total / (1024 * 1024),
		AvailableMB: v.Available / (1024 * 1024),
		UsedPercent: v.UsedPercent,
	}
}

func getProcessRSSMB() uint64 {
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return 0
	}
	mi, err := p.MemoryInfo()
	if err != nil {
		return 0
	}
	return mi.RSS / (1024 * 1024)
}

func getDiskFreeMB(path string) uint64 {
	if path == "" {
		return 0
	}
	usage, err := disk.Usage(path)
	if err != nil {
		return 0
	}
	return usage.Free / (1024 * 1024)
}

func (s *Service) profilesDir() string {
	return initialparams.Get().Paths.ProfilesDir
}

func (s *Service) getStatJSON() string {
	if s.app == nil {
		return ""
	}
	svc, err := app.GetComponent[debugstat.StatService](s.app)
	if err != nil {
		return ""
	}
	data, err := json.Marshal(svc.GetStat())
	if err != nil {
		return ""
	}
	return string(data)
}

// profileInfo is written as info.json inside profile zip archives.
type profileInfo struct {
	Version      string           `json:"version"`
	Reason       string           `json:"reason,omitempty"`
	ReasonDesc   string           `json:"reasonDesc,omitempty"`
	Time         string           `json:"time"`
	Platform     string           `json:"platform"`
	NumCPU       int              `json:"numCPU"`
	CPUModel     string           `json:"cpuModel,omitempty"`
	ProcessRSSMB uint64           `json:"processRSSMB"`
	DiskFreeMB   uint64           `json:"diskFreeMB"`
	MemStats     runtime.MemStats `json:"memstats"`
	SystemMemory *systemMemory    `json:"systemMemory,omitempty"`
	// PeerId and AccountId are populated only when an account is active;
	// a snapshot taken before AccountSelect leaves them empty.
	PeerId    string `json:"peerId,omitempty"`
	AccountId string `json:"accountId,omitempty"`
}

// newProfileZip wraps w in a zip.Writer configured with BestSpeed Deflate,
// matching the compression settings used for every profile archive.
func newProfileZip(w io.Writer) *zip.Writer {
	zipw := zip.NewWriter(w)
	zipw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestSpeed)
	})
	return zipw
}

// buildProfileInfo collects runtime, host, and identity fields for the
// info.json entry of a profile archive. reason and reasonDesc come from the
// originating RPC; the rest is derived.
func (s *Service) buildProfileInfo(reason, reasonDesc string) profileInfo {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	peerId, accountId := s.identity()
	return profileInfo{
		Version:      vcs.GetVCSInfo().Version(),
		Reason:       reason,
		ReasonDesc:   reasonDesc,
		Time:         time.Now().Format(time.RFC3339),
		Platform:     runtime.GOOS + "/" + runtime.GOARCH,
		NumCPU:       runtime.NumCPU(),
		CPUModel:     getCPUModel(),
		ProcessRSSMB: getProcessRSSMB(),
		DiskFreeMB:   getDiskFreeMB(s.rootPath),
		MemStats:     ms,
		SystemMemory: getSystemMemory(),
		PeerId:       peerId,
		AccountId:    accountId,
	}
}

// writeProfileMetadata writes info.json and (when non-empty) stat.json into
// the zip archive.
func writeProfileMetadata(zipw *zip.Writer, info profileInfo, statJSON string) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal info: %w", err)
	}
	w, err := zipw.Create("info.json")
	if err != nil {
		return fmt.Errorf("create info entry: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write info: %w", err)
	}
	if statJSON == "" {
		return nil
	}
	w, err = zipw.Create("stat.json")
	if err != nil {
		return fmt.Errorf("create stat entry: %w", err)
	}
	if _, err := w.Write([]byte(statJSON)); err != nil {
		return fmt.Errorf("write stat: %w", err)
	}
	return nil
}

// identity returns the current device PeerId and Account Id if the app is
// running and a wallet component is available; otherwise returns empty
// strings. Safe to call before AccountSelect.
func (s *Service) identity() (peerId, accountId string) {
	if s == nil || s.app == nil {
		return "", ""
	}
	w, ok := s.app.Component(walletComp.CName).(walletComp.Wallet)
	if !ok || w == nil {
		return "", ""
	}
	if dk := w.GetDevicePrivkey(); dk != nil {
		peerId = dk.GetPublic().PeerId()
	}
	if ak := w.GetAccountPrivkey(); ak != nil {
		accountId = ak.GetPublic().Account()
	}
	return
}

type systemMemory struct {
	TotalMB     uint64  `json:"totalMB"`
	AvailableMB uint64  `json:"availableMB"`
	UsedPercent float64 `json:"usedPercent"`
}

// SaveDebugSnapshot saves heap profile, goroutine stacks, and runtime info
// as a zip file. Called via DebugRunProfiler with DurationInSeconds=0.
func (s *Service) SaveDebugSnapshot(reason, reasonDesc string) (string, error) {
	info := s.buildProfileInfo(reason, reasonDesc)
	statJSON := s.getStatJSON()
	profilesDir := s.profilesDir()
	if profilesDir == "" {
		return "", fmt.Errorf("log path not configured")
	}
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		return "", fmt.Errorf("create profiles dir: %w", err)
	}

	ts := time.Now().Format("20060102_150405")
	zipPath := filepath.Join(profilesDir, fmt.Sprintf("snapshot_%s.zip", ts))

	zipF, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("create zip: %w", err)
	}
	defer zipF.Close()

	zipw := newProfileZip(zipF)

	if err := writeProfileMetadata(zipw, info, statJSON); err != nil {
		zipw.Close()
		return "", err
	}

	// Heap profile
	w, err := zipw.Create("heap.pb.gz")
	if err != nil {
		zipw.Close()
		return "", fmt.Errorf("create heap entry: %w", err)
	}
	if err := pprof.WriteHeapProfile(w); err != nil {
		zipw.Close()
		return "", fmt.Errorf("write heap: %w", err)
	}

	// Goroutine stacks
	w, err = zipw.Create("goroutines.txt")
	if err != nil {
		zipw.Close()
		return "", fmt.Errorf("create goroutines entry: %w", err)
	}
	if _, err := w.Write(debug.Stack(true)); err != nil {
		zipw.Close()
		return "", fmt.Errorf("write goroutines: %w", err)
	}

	if err := zipw.Close(); err != nil {
		return "", fmt.Errorf("close zip: %w", err)
	}

	pruneOldProfilesIfOvercrowded(profilesDir, profilesPruneTrigger, profilesPruneOlderThan)
	return zipPath, nil
}

// profilesPruneTrigger is the file count that must be exceeded before any
// age-based pruning runs. Below this count every profile is kept regardless
// of age; above it, files older than profilesPruneOlderThan are removed.
const profilesPruneTrigger = 50
const profilesPruneOlderThan = 30 * 24 * time.Hour

// pruneOldProfilesIfOvercrowded is a lazy, trigger-based cleanup for the
// profiles directory. It is NOT a hard cap — below countTrigger nothing is
// deleted even if entries are older than maxAge, and above countTrigger only
// entries older than maxAge are removed (so the directory can still exceed
// countTrigger if every file is fresh). The intent is "let the directory
// breathe, but don't hoard stale files once it starts filling up".
func pruneOldProfilesIfOvercrowded(dir string, countTrigger int, maxAge time.Duration) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if len(entries) <= countTrigger {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func (s *Service) RunProfiler(ctx context.Context, seconds int, reason, reasonDesc string) (string, error) {
	if seconds == 0 {
		return s.SaveDebugSnapshot(reason, reasonDesc)
	}

	profilesDir := s.profilesDir()
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		return "", fmt.Errorf("create profiles dir: %w", err)
	}

	// In-flight trace (already in memory from recorder)
	inFlightTraceBuf, err := s.traceRecorder.stopAndGetInFlightTrace()
	if err != nil {
		return "", err
	}

	// Create temp files for streaming. We defer removal unconditionally —
	// the caller never sees these paths, only the final zip.
	type tempFile struct {
		zipName string
		path    string
	}
	var temps []tempFile
	defer func() {
		for _, t := range temps {
			os.Remove(t.path)
		}
	}()

	createTemp := func(zipName, pattern string) (*os.File, error) {
		f, err := os.CreateTemp(profilesDir, pattern)
		if err != nil {
			return nil, err
		}
		temps = append(temps, tempFile{zipName: zipName, path: f.Name()})
		return f, nil
	}

	// Stream trace to file
	traceF, err := createTemp("trace", "tmp_trace_*")
	if err != nil {
		return "", fmt.Errorf("create trace file: %w", err)
	}
	if err := trace.Start(traceF); err != nil {
		traceF.Close()
		return "", fmt.Errorf("start tracer: %w", err)
	}
	traceRunning := true
	defer func() {
		if traceRunning {
			trace.Stop()
		}
		traceF.Close()
	}()

	// Stream CPU profile to file
	cpuF, err := createTemp("cpu_profile", "tmp_cpu_*")
	if err != nil {
		return "", fmt.Errorf("create cpu file: %w", err)
	}
	if err := pprof.StartCPUProfile(cpuF); err != nil {
		cpuF.Close()
		return "", fmt.Errorf("start cpu profile: %w", err)
	}
	cpuRunning := true
	defer func() {
		if cpuRunning {
			pprof.StopCPUProfile()
		}
		cpuF.Close()
	}()

	// Heap start - stream to file
	heapStartF, err := createTemp("heap_start", "tmp_heap_start_*")
	if err != nil {
		return "", fmt.Errorf("create heap start file: %w", err)
	}
	err = pprof.WriteHeapProfile(heapStartF)
	heapStartF.Close()
	if err != nil {
		return "", fmt.Errorf("write heap start: %w", err)
	}

	// Goroutines start - stream to file (reuse stackBuf for end)
	var stackBuf []byte
	gsF, err := createTemp("goroutines_start.txt", "tmp_goroutines_start_*")
	if err != nil {
		return "", fmt.Errorf("create goroutines start file: %w", err)
	}
	stackBuf = debug.StackReuse(stackBuf, true)
	_, err = gsF.Write(stackBuf)
	gsF.Close()
	if err != nil {
		return "", fmt.Errorf("write goroutines start: %w", err)
	}

	// Wait
	select {
	case <-time.After(time.Duration(seconds) * time.Second):
	case <-ctx.Done():
	}

	// Stop profilers so the on-disk files are finalized before we read them.
	// The deferred cleanup above still closes the underlying files.
	pprof.StopCPUProfile()
	cpuRunning = false
	trace.Stop()
	traceRunning = false

	// Heap end
	heapEndF, err := createTemp("heap_end", "tmp_heap_end_*")
	if err != nil {
		return "", fmt.Errorf("create heap end file: %w", err)
	}
	err = pprof.WriteHeapProfile(heapEndF)
	heapEndF.Close()
	if err != nil {
		return "", fmt.Errorf("write heap end: %w", err)
	}

	// Goroutines end (reuse stackBuf from start)
	geF, err := createTemp("goroutines_end.txt", "tmp_goroutines_end_*")
	if err != nil {
		return "", fmt.Errorf("create goroutines end file: %w", err)
	}
	stackBuf = debug.StackReuse(stackBuf, true)
	_, err = geF.Write(stackBuf)
	geF.Close()
	if err != nil {
		return "", fmt.Errorf("write goroutines end: %w", err)
	}

	// Pack into zip, streaming each temp file from disk. The zip path and
	// zip writer are both managed through defers: on any error before
	// zipSuccess flips, the partial archive is removed.
	zipPath := filepath.Join(profilesDir, fmt.Sprintf("anytype_profile_%s.zip", time.Now().Format("20060102_150405")))
	zipF, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("create zip: %w", err)
	}
	zipSuccess := false
	defer func() {
		zipF.Close()
		if !zipSuccess {
			os.Remove(zipPath)
		}
	}()
	zipw := newProfileZip(zipF)
	defer zipw.Close()

	for _, t := range temps {
		src, err := os.Open(t.path)
		if err != nil {
			return "", fmt.Errorf("open temp file %s: %w", t.zipName, err)
		}
		dst, err := zipw.Create(t.zipName)
		if err != nil {
			src.Close()
			return "", fmt.Errorf("create zip entry %s: %w", t.zipName, err)
		}
		_, err = io.Copy(dst, src)
		src.Close()
		if err != nil {
			return "", fmt.Errorf("copy %s to zip: %w", t.zipName, err)
		}
	}
	if inFlightTraceBuf != nil {
		dst, err := zipw.Create("account_select_trace")
		if err != nil {
			return "", fmt.Errorf("write in-flight trace: %w", err)
		}
		if _, err := io.Copy(dst, inFlightTraceBuf); err != nil {
			return "", fmt.Errorf("write in-flight trace: %w", err)
		}
	}

	if err := writeProfileMetadata(zipw, s.buildProfileInfo(reason, reasonDesc), s.getStatJSON()); err != nil {
		return "", err
	}

	zipSuccess = true
	pruneOldProfilesIfOvercrowded(profilesDir, profilesPruneTrigger, profilesPruneOlderThan)
	return zipPath, nil
}

type zipFile struct {
	name    string
	data    io.Reader
	modTime time.Time // zero value means "use archive creation time"
}

func createZipArchive(w io.Writer, files []zipFile) error {
	zipw := zip.NewWriter(w)
	zipw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestSpeed)
	})
	err := func() error {
		for _, file := range files {
			header := &zip.FileHeader{
				Name:   file.name,
				Method: zip.Deflate,
			}
			if !file.modTime.IsZero() {
				header.Modified = file.modTime
			}
			f, err := zipw.CreateHeader(header)
			if err != nil {
				return fmt.Errorf("create file in zip archive: %w", err)
			}
			_, err = io.Copy(f, file.data)
			if err != nil {
				return fmt.Errorf("copy data to file: %w", err)
			}
		}
		return nil
	}()
	return errors.Join(err, zipw.Close())
}

func (s *Service) SaveLoginTrace(dir string) (string, error) {
	return s.traceRecorder.save(dir)
}

// buildPartialLogBundle writes a capped gzip bundle of the logs in logsDir
// to a temp file and returns its path plus the newest source mtime. An empty
// path is returned when there are no eligible log files.
func (s *Service) buildPartialLogBundle(logsDir string) (string, time.Time, error) {
	var newest time.Time
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", time.Time{}, nil
		}
		return "", time.Time{}, fmt.Errorf("read logs dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "anytype") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if t := info.ModTime(); t.After(newest) {
			newest = t
		}
	}
	if newest.IsZero() {
		return "", time.Time{}, nil
	}
	tmp, err := os.CreateTemp("", "anytype-logs-*.log.gz")
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create tmp bundle: %w", err)
	}
	tmp.Close()
	if err := logging.WriteLogBundle(logsDir, tmp.Name(), partialReportLogBudget); err != nil {
		os.Remove(tmp.Name())
		return "", time.Time{}, err
	}
	return tmp.Name(), newest, nil
}

// partialReportLogBudget caps the size of the single gzipped log bundle
// produced when SaveReport is called with full=false. Bundle is streamed
// newest-first across active + rotated logs until the cap is hit.
const partialReportLogBudget = 10 * 1024 * 1024

// SaveReport creates a zip of logs and profiles. Returns (path, summary JSON, lastModifiedTs, error).
// lastModifiedTs is the Unix timestamp (seconds) of the most recently modified source file included in the report.
// When full is false the report replaces the individual log files with a
// single gzipped bundle capped at partialReportLogBudget; profiles are
// included in both modes.
func (s *Service) SaveReport(destDir string, full bool) (string, string, int64, error) {
	paths := initialparams.Get().Paths
	logsDir := paths.LogsDir
	if logsDir == "" {
		return "", "", 0, ErrNoFolder
	}
	namePattern := fmt.Sprintf("anytype-report-%s-*.zip", time.Now().Format("20060102-150405"))
	targetFile, err := os.CreateTemp(destDir, namePattern)
	if err != nil {
		return "", "", 0, fmt.Errorf("create temp file: %w", err)
	}

	err = os.Chmod(targetFile.Name(), 0664)
	if err != nil {
		return "", "", 0, err
	}

	// Drain zap's buffered sink so log entries written right before the
	// report call make it to disk before we start reading the log files.
	// Error is benign (stderr Sync can fail on some platforms) — ignored.
	_ = log.Sync()

	var files []zipFile
	var toClose []io.Closer
	var lastModifiedTs int64

	// parentDir is the "common" dir; zip entries are relative to it so the
	// archive preserves the logs/ and profiles/ subdirectory structure.
	parentDir := filepath.Dir(logsDir)

	collectDir := func(dir string, filter func(string) bool) ([]zipFile, error) {
		var collected []zipFile
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("error accessing file %s: %w", path, err)
			}
			if info.IsDir() {
				return nil
			}
			if filter != nil && !filter(info.Name()) {
				return nil
			}
			if ts := info.ModTime().Unix(); ts > lastModifiedTs {
				lastModifiedTs = ts
			}
			relPath, err := filepath.Rel(parentDir, path)
			if err != nil {
				return fmt.Errorf("failed to compute relative path: %w", err)
			}
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			collected = append(collected, zipFile{name: relPath, data: file, modTime: info.ModTime()})
			toClose = append(toClose, file)
			return nil
		})
		return collected, err
	}

	if full {
		logFiles, err := collectDir(logsDir, func(name string) bool {
			return strings.HasPrefix(name, "anytype")
		})
		if err != nil {
			return "", "", 0, fmt.Errorf("error while walking logs directory: %w", err)
		}
		files = append(files, logFiles...)
	} else {
		// Partial report: one capped gzip bundle in place of the individual
		// log files. Track newest log mtime separately so lastModifiedTs
		// reflects it without loading every file into the zip.
		bundlePath, bundleMTime, err := s.buildPartialLogBundle(logsDir)
		if err != nil {
			return "", "", 0, fmt.Errorf("build log bundle: %w", err)
		}
		if bundlePath != "" {
			bundleFile, err := os.Open(bundlePath)
			if err != nil {
				os.Remove(bundlePath)
				return "", "", 0, fmt.Errorf("open log bundle: %w", err)
			}
			toClose = append(toClose, bundleFile)
			// Delete the temp bundle after the zip is built.
			defer os.Remove(bundlePath)
			files = append(files, zipFile{
				name:    filepath.Join(filepath.Base(logsDir), "bundle.log.gz"),
				data:    bundleFile,
				modTime: bundleMTime,
			})
			if ts := bundleMTime.Unix(); ts > lastModifiedTs {
				lastModifiedTs = ts
			}
		}
	}

	// Collect profile files
	profilesDir := paths.ProfilesDir
	if profilesDir != "" {
		if info, statErr := os.Stat(profilesDir); statErr == nil && info.IsDir() {
			profileFiles, walkErr := collectDir(profilesDir, nil)
			if walkErr != nil {
				return "", "", 0, fmt.Errorf("error while walking profiles directory: %w", walkErr)
			}
			files = append(files, profileFiles...)
		}
	}
	defer func() {
		for _, closeable := range toClose {
			closeable.Close()
		}
	}()

	if len(files) == 0 {
		return "", "", 0, errors.New("no log files found in directory")
	}

	// Generate profiles summary
	var summaryStr string
	if profilesDir != "" {
		if summary := generateProfilesSummary(profilesDir, logsDir); len(summary) > 0 {
			summaryStr = string(summary)
			files = append(files, zipFile{name: "profiles_summary.json", data: bytes.NewReader(summary), modTime: time.Now()})
		}
	}

	err = createZipArchive(targetFile, files)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create zip archive: %w", err)
	}

	return targetFile.Name(), summaryStr, lastModifiedTs, targetFile.Close()
}

// CleanupReport removes log and profile source files with a modification time strictly before ts (Unix seconds).
func (s *Service) CleanupReport(ts int64) error {
	paths := initialparams.Get().Paths
	if paths.LogsDir == "" {
		return ErrNoFolder
	}
	cutoff := time.Unix(ts, 0)

	removeOld := func(dir string, filter func(string) bool) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("read dir %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if filter != nil && !filter(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
		return nil
	}

	if err := removeOld(paths.LogsDir, func(name string) bool {
		return strings.HasPrefix(name, "anytype")
	}); err != nil {
		return err
	}

	if paths.ProfilesDir != "" {
		if err := removeOld(paths.ProfilesDir, nil); err != nil {
			return err
		}
	}
	return nil
}

type profilesSummary struct {
	Profiles     int                  `json:"profiles"`
	LongRequests int                  `json:"longRequests"`
	Logs         int                  `json:"logs"`
	ReasonCounts map[string]int       `json:"reasonCounts"`
	Items        []profileSummaryItem `json:"items,omitempty"`
}

// profileSummaryItem captures the per-snapshot fields pulled from each
// profile zip's info.json, so a reviewer can see the contents of a report
// without opening every archive.
type profileSummaryItem struct {
	File       string `json:"file"`
	Version    string `json:"version,omitempty"`
	Reason     string `json:"reason,omitempty"`
	ReasonDesc string `json:"reasonDesc,omitempty"`
	Time       string `json:"time,omitempty"`
}

func generateProfilesSummary(profilesDir, logsDir string) []byte {
	summary := profilesSummary{
		ReasonCounts: make(map[string]int),
	}

	if entries, err := os.ReadDir(profilesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			switch {
			case strings.HasSuffix(name, ".zip"):
				summary.Profiles++
				item := profileSummaryItem{File: name}
				if info, ok := readInfoFromZip(filepath.Join(profilesDir, name)); ok {
					reason := info.Reason
					if reason == "" {
						reason = "(none)"
					}
					summary.ReasonCounts[reason]++
					item.Version = info.Version
					item.Reason = info.Reason
					item.ReasonDesc = info.ReasonDesc
					item.Time = info.Time
				}
				summary.Items = append(summary.Items, item)
			case strings.HasPrefix(name, metrics.LongMethodTracePrefix):
				summary.LongRequests++
			}
		}
	}

	// Count log files
	if entries, err := os.ReadDir(logsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				summary.Logs++
			}
		}
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return nil
	}
	return data
}

func readInfoFromZip(zipPath string) (profileInfo, bool) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return profileInfo{}, false
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != "info.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return profileInfo{}, false
		}
		defer rc.Close()
		var info profileInfo
		if err := json.NewDecoder(rc).Decode(&info); err != nil {
			return profileInfo{}, false
		}
		return info, true
	}
	return profileInfo{}, false
}

// traceRecorder is a helper to start and stop flight trace recorder
type traceRecorder struct {
	lock            sync.Mutex
	recorder        *exptrace.FlightRecorder
	lastRecordedBuf *bytes.Buffer // contains zip archive of trace
}

// empty dir means use system temp dir
func (r *traceRecorder) save(dir string) (string, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	var traceReader io.Reader
	if r.recorder == nil {
		if r.lastRecordedBuf == nil {
			return "", errors.New("no running trace recorder")
		}
		traceReader = r.lastRecordedBuf
		r.lastRecordedBuf = nil
	} else {
		buf := bytes.NewBuffer(nil)
		err := r.saveTraceToZipArchive(buf)
		if err != nil {
			return "", fmt.Errorf("save trace to zip archive: %w", err)
		}
		traceReader = buf
	}

	f, err := os.CreateTemp(dir, "account-select-trace-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	_, err = io.Copy(f, traceReader)
	if err != nil {
		return "", errors.Join(f.Close(), fmt.Errorf("copy trace: %w", err))
	}
	return f.Name(), f.Close()
}

func (r *traceRecorder) start() {
	flightRecorder := exptrace.NewFlightRecorder()
	flightRecorder.SetPeriod(60 * time.Second)
	err := flightRecorder.Start()
	if err == nil {
		r.lock.Lock()
		r.recorder = flightRecorder
		r.lock.Unlock()
	}
}

func (r *traceRecorder) stop() {
	r.lock.Lock()
	if r.recorder != nil {
		r.lastRecordedBuf = bytes.NewBuffer(nil)
		// Store trace in memory as zip archive to reduce memory usage
		err := r.saveTraceToZipArchive(r.lastRecordedBuf)
		if err != nil {
			log.With("error", err).Error("save trace to zip archive")
		}
		err = r.recorder.Stop()
		if err != nil {
			log.With("error", err).Error("stop trace recorder")
		}
		r.recorder = nil
	}
	r.lock.Unlock()
}

func (r *traceRecorder) saveTraceToZipArchive(w io.Writer) error {
	buf := bytes.NewBuffer(nil)
	_, err := r.recorder.WriteTo(buf)
	if err != nil {
		return fmt.Errorf("write trace: %w", err)
	}
	err = createZipArchive(w, []zipFile{{name: "account_select_trace", data: buf}})
	if err != nil {
		return fmt.Errorf("create zip archive: %w", err)
	}
	return nil
}

func (r *traceRecorder) stopAndGetInFlightTrace() (*bytes.Buffer, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.recorder != nil {
		buf := bytes.NewBuffer(nil)
		_, err := r.recorder.WriteTo(buf)
		if err != nil {
			return nil, fmt.Errorf("write in-flight trace: %w", err)
		}
		err = r.recorder.Stop()
		if err != nil {
			log.With("error", err).Error("stop trace recorder")
		}
		r.recorder = nil
		return buf, nil
	}
	return nil, nil
}
