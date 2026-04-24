// Package debugsnapshot produces the "heap profile + goroutine stacks +
// runtime info" zip archive that the DebugRunProfiler RPC (duration=0) and
// the desktop memory-growth detector both emit. It lives outside
// core/application so components that can't reach application.Service (for
// example the profiler background goroutine) can still write a snapshot.
package debugsnapshot

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/klauspost/compress/flate"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/anyproto/anytype-heart/util/debug"
	"github.com/anyproto/anytype-heart/util/vcs"
)

// Info is serialized as info.json inside every snapshot archive.
type Info struct {
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
	SystemMemory *SystemMemory    `json:"systemMemory,omitempty"`
	// PeerId and AccountId are populated only when an account is active;
	// snapshots taken before AccountSelect (or by background components that
	// don't have access to the wallet) leave them empty.
	PeerId    string `json:"peerId,omitempty"`
	AccountId string `json:"accountId,omitempty"`
}

// SystemMemory carries OS-reported virtual memory stats.
type SystemMemory struct {
	TotalMB     uint64  `json:"totalMB"`
	AvailableMB uint64  `json:"availableMB"`
	UsedPercent float64 `json:"usedPercent"`
}

// Meta carries the optional context a caller wants embedded in the snapshot.
// All fields may be empty when the caller doesn't have the information.
type Meta struct {
	StatJSON  string // raw JSON to be saved as stat.json; "" omits the entry
	PeerId    string
	AccountId string
	RootPath  string // used for disk-free calculation
}

// Save writes a new snapshot archive into profilesDir and returns its path.
// The caller must supply a non-empty profilesDir (empty returns an error that
// the callers recognize as "log path not configured").
func Save(profilesDir, reason, reasonDesc string, meta Meta) (string, error) {
	if profilesDir == "" {
		return "", fmt.Errorf("log path not configured")
	}
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		return "", fmt.Errorf("create profiles dir: %w", err)
	}

	info := BuildInfo(reason, reasonDesc, meta)

	ts := time.Now().Format("20060102_150405")
	zipPath := filepath.Join(profilesDir, fmt.Sprintf("snapshot_%s.zip", ts))

	zipF, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("create zip: %w", err)
	}
	defer zipF.Close()

	zipw := NewZipWriter(zipF)

	if err := WriteMetadata(zipw, info, meta.StatJSON); err != nil {
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
	return zipPath, nil
}

// BuildInfo collects runtime, host, and identity fields for the info.json
// entry of a snapshot. Exported so the timed-profile path (RunProfiler) can
// reuse the same data shape for its own zip.
func BuildInfo(reason, reasonDesc string, meta Meta) Info {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return Info{
		Version:      vcs.GetVCSInfo().Version(),
		Reason:       reason,
		ReasonDesc:   reasonDesc,
		Time:         time.Now().Format(time.RFC3339),
		Platform:     runtime.GOOS + "/" + runtime.GOARCH,
		NumCPU:       runtime.NumCPU(),
		CPUModel:     getCPUModel(),
		ProcessRSSMB: getProcessRSSMB(),
		DiskFreeMB:   getDiskFreeMB(meta.RootPath),
		MemStats:     ms,
		SystemMemory: getSystemMemory(),
		PeerId:       meta.PeerId,
		AccountId:    meta.AccountId,
	}
}

// NewZipWriter wraps w with a zip writer configured to use BestSpeed Deflate —
// the shared compression setting for every profile archive.
func NewZipWriter(w io.Writer) *zip.Writer {
	zipw := zip.NewWriter(w)
	zipw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestSpeed)
	})
	return zipw
}

// WriteMetadata writes info.json and (when non-empty) stat.json into zipw.
// Exposed so RunProfiler can reuse the exact metadata shape.
func WriteMetadata(zipw *zip.Writer, info Info, statJSON string) error {
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

// ReadInfoFromZip extracts and decodes info.json from a snapshot archive.
// Returns (zero, false) if the archive can't be opened, doesn't contain
// info.json, or the JSON fails to parse — callers use this to decide whether
// a reason/version is available without blowing up the whole summary step.
func ReadInfoFromZip(zipPath string) (Info, bool) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return Info{}, false
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != "info.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return Info{}, false
		}
		defer rc.Close()
		var info Info
		if err := json.NewDecoder(rc).Decode(&info); err != nil {
			return Info{}, false
		}
		return info, true
	}
	return Info{}, false
}

// --- host helpers ---

func getCPUModel() string {
	infos, err := cpu.Info()
	if err != nil || len(infos) == 0 {
		return ""
	}
	return infos[0].ModelName
}

func getSystemMemory() *SystemMemory {
	v, err := mem.VirtualMemory()
	if err != nil {
		return nil
	}
	return &SystemMemory{
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
