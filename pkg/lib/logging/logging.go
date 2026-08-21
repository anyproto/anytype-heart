package logging

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/anyproto/anytype-heart/pkg/lib/initialparams"
)

const DefaultLogLevels = "common.commonspace.headsync=INFO;core.block.editor.spaceview=INFO;anytype-app=INFO;anytype-core-account=INFO;*=WARN"
const lumberjackScheme = "lumberjack"

var DefaultCfg = logger.Config{
	Production:   false,
	DefaultLevel: "WARN",
	Format:       logger.JSONOutput,
}

func Logger(system string) *Sugared {
	return &Sugared{logger.NewNamedSugared(system)}
}

func LoggerNotSugared(system string) *zap.Logger {
	lg := logger.NewNamed(system)

	return lg.Logger
}

// LevelsFromStr parses a string of the form "name1=DEBUG;prefix*=WARN;*=ERROR" into a slice of NamedLevel
// it may be useful to parse the log level from the OS env var
func LevelsFromStr(s string) (levels []logger.NamedLevel) {
	for _, kv := range strings.Split(s, ";") {
		if kv == "" {
			continue
		}
		strings.TrimSpace(kv)
		parts := strings.Split(kv, "=")
		var key, value string
		if len(parts) == 1 {
			key = "*"
			value = strings.TrimSpace(parts[0])
		} else if len(parts) == 2 {
			key = strings.TrimSpace(parts[0])
			value = strings.TrimSpace(parts[1])
		} else {
			fmt.Printf("invalid log level format. It should be something like `prefix*=LEVEL;*suffix=LEVEL`, where LEVEL is one of valid log levels\n")
			continue
		}
		if key == "" || value == "" {
			continue
		}

		_, err := zap.ParseAtomicLevel(value)
		if err != nil {
			fmt.Printf("Can't parse log level %s: %s\n", parts[0], err)
			continue
		}
		levels = append(levels, logger.NamedLevel{Name: key, Level: value})
	}
	return levels
}
func SetLogLevels(levels string) {
	cfg := DefaultCfg

	cfg.Levels = LevelsFromStr(levels)
	cfg.ApplyGlobal()
}

type lumberjackSink struct {
	*lumberjack.Logger
}

// Sync is a no-op: we rely on the OS page cache to flush eventually and
// avoid the fsync cost on every log write.
func (s *lumberjackSink) Sync() error {
	return nil
}

// bufferedLumberjackSink wraps a lumberjack rotator with an in-memory buffer so
// many small log writes coalesce into a single syscall per flush.
type bufferedLumberjackSink struct {
	*zapcore.BufferedWriteSyncer
	lj *lumberjack.Logger
}

func (s *bufferedLumberjackSink) Close() error {
	// Stop flushes pending buffered bytes to the underlying writer and stops
	// the background flush goroutine.
	stopErr := s.BufferedWriteSyncer.Stop()
	closeErr := s.lj.Close()
	return errors.Join(stopErr, closeErr)
}

// activeSink is the most recently registered lumberjack sink. Tracked so
// CloseSink (called on shutdown) can stop the background flush goroutine
// and close the underlying file deterministically. Access is guarded by
// the package-level zap registration which only happens once.
var activeSink *bufferedLumberjackSink

// sinkFilename extracts the log file path from a sink URL.
//
// zap resolves output paths through url.Parse. A Windows path carries a drive
// letter, so "lumberjack:C:\dir\anytype.log" has no "/" after the scheme and is
// parsed as an *opaque* URL: the whole path lands in u.Opaque and u.Path is
// empty. Using u.Path alone therefore yielded an empty Filename on Windows,
// and lumberjack silently fell back to os.TempDir()/<argv0>-lumberjack.log —
// so no logs ever appeared in the expected directory and debug reports shipped
// with zero log files. This is not a path-separator issue: "C:/dir/x.log"
// parses as opaque too.
func sinkFilename(u *url.URL) string {
	if u.Path != "" {
		return u.Path
	}
	return u.Opaque
}

func newLumberjackSink(u *url.URL) (zap.Sink, error) {
	var lj *lumberjack.Logger
	const bufSize = 256 * 1024
	const flushInterval = 30 * time.Second
	filename := sinkFilename(u)
	// Mobile keeps smaller rotated files to save disk; buffering behaviour
	// matches desktop.
	if runtime.GOOS == "android" || runtime.GOOS == "ios" {
		lj = &lumberjack.Logger{
			Filename:   filename,
			MaxSize:    10,
			MaxBackups: 2,
			Compress:   false,
		}
	} else {
		lj = &lumberjack.Logger{
			Filename:   filename,
			MaxSize:    100,
			MaxBackups: 10,
			Compress:   true,
		}
	}
	sink := &bufferedLumberjackSink{
		BufferedWriteSyncer: &zapcore.BufferedWriteSyncer{
			WS:            &lumberjackSink{Logger: lj},
			Size:          bufSize,
			FlushInterval: flushInterval,
		},
		lj: lj,
	}
	activeSink = sink
	return sink, nil
}

// CloseSink stops the background flush goroutine of the lumberjack sink and
// closes the underlying log file. Safe to call when no sink is registered
// (no-op). Bounded by timeout so a stuck disk can't hang shutdown — if the
// stop takes longer, we abandon and let the OS reclaim the FD on exit.
func CloseSink(timeout time.Duration) error {
	sink := activeSink
	if sink == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() {
		done <- sink.Close()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("close lumberjack sink: timed out after %s", timeout)
	}
}

// SyncWithTimeout drains zap's buffered sink so recent log lines reach the
// OS page cache before process exit. Bounded — a stuck disk should not stall
// shutdown indefinitely. Errors are returned but typically benign (stderr
// Sync can fail on some platforms).
func SyncWithTimeout(timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- DefaultLogger().Sync()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("zap sync: timed out after %s", timeout)
	}
}

// DefaultLogger returns the global zap logger. Helper kept here so callers
// don't need to import go.uber.org/zap directly for shutdown plumbing.
func DefaultLogger() *zap.Logger {
	return zap.L()
}

// Init configures global zap levels and, when saveLogs is true, routes logs to
// a rotating file under initialparams.Get().Paths.LogFile. Callers must call
// initialparams.Init first so the path layout is resolved.
func Init(logLevels string, saveLogs bool) {
	paths := initialparams.Get().Paths
	if paths.Common != "" {
		if err := os.MkdirAll(paths.Common, 0755); err != nil && !os.IsExist(err) {
			fmt.Println("failed to create common dir", err)
		}
	}

	if !saveLogs {
		DefaultCfg.Format = logger.ColorizedOutput
	} else {
		registerLumberjackSink(paths.LogsDir, paths.LogFile, &DefaultCfg)
	}
	envLogLevels := os.Getenv("ANYTYPE_LOG_LEVEL")
	if logLevels == "" {
		logLevels = envLogLevels
	}
	if logLevels == "" {
		logLevels = DefaultLogLevels
	}

	SetLogLevels(logLevels)
}

func registerLumberjackSink(logsDir, logFile string, config *logger.Config) {
	if logsDir == "" || logFile == "" {
		fmt.Println("logs dir is not set")
		return
	}
	err := os.Mkdir(logsDir, 0755)
	if err != nil && !os.IsExist(err) {
		fmt.Println("failed to create logs dir", err)
		return
	}

	err = zap.RegisterSink(lumberjackScheme, newLumberjackSink)
	if err != nil {
		fmt.Println("failed to register lumberjack sink", err)
	}

	config.AddOutputPaths = append(config.AddOutputPaths, lumberjackScheme+":"+logFile)
}
