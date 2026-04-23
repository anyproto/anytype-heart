// Package initialparams holds process-wide parameters supplied by the client
// through the InitialSetParameters RPC. It is the single source of truth for
// the on-disk layout of side files (logs, profiles) so that the logger, the
// profiler, and report RPCs all derive paths from the same place.
//
// The logger is initialized before the app container is built, so these
// parameters cannot live on an app component. Init is expected to be called
// exactly once at process start; subsequent calls replace the stored value.
package initialparams

import (
	"errors"
	"path/filepath"
	"sync/atomic"

	"github.com/anyproto/anytype-heart/pb"
)

// ErrAlreadyInitialized is returned by Init when the process-global Params
// have already been stored. Callers must not re-run startup side effects
// (logging, metrics registration, etc.) in that case.
var ErrAlreadyInitialized = errors.New("initial parameters already set")

// Paths is the resolved on-disk layout rooted at the client-supplied workdir.
// All derived directories share the "common" subdirectory so that per-account
// storage stays separate from process-scoped side files.
type Paths struct {
	Workdir     string // raw path from the client
	Common      string // <workdir>/common
	LogsDir     string // <workdir>/common/logs
	LogFile     string // <workdir>/common/logs/anytype.log
	ProfilesDir string // <workdir>/common/profiles
}

// Params captures everything the client sends in InitialSetParameters.
type Params struct {
	Platform      string
	Version       string
	LogLevel      string
	SaveLogs      bool
	SendTelemetry bool
	Paths         Paths
}

var current atomic.Pointer[Params]

// Init computes and stores Params derived from the incoming RPC request.
// Returns the stored Params on success. If Init has already been called,
// returns the previously stored Params and ErrAlreadyInitialized; callers
// must not re-run startup side effects in that case.
func Init(req *pb.RpcInitialSetParametersRequest) (Params, error) {
	p := Params{
		Platform:      req.Platform,
		Version:       req.Version,
		LogLevel:      req.LogLevel,
		SaveLogs:      !req.DoNotSaveLogs,
		SendTelemetry: !req.DoNotSendTelemetry,
		Paths:         paths(req.Workdir),
	}
	if current.CompareAndSwap(nil, &p) {
		return p, nil
	}
	return *current.Load(), ErrAlreadyInitialized
}

// Get returns the stored Params, or the zero value if Init has not been
// called yet. Reading before Init is legal but uninformative: consumers must
// tolerate empty path fields.
func Get() Params {
	if p := current.Load(); p != nil {
		return *p
	}
	return Params{}
}

func paths(workdir string) Paths {
	if workdir == "" {
		return Paths{}
	}
	common := filepath.Join(workdir, "common")
	logs := filepath.Join(common, "logs")
	return Paths{
		Workdir:     workdir,
		Common:      common,
		LogsDir:     logs,
		LogFile:     filepath.Join(logs, "anytype.log"),
		ProfilesDir: filepath.Join(common, "profiles"),
	}
}
