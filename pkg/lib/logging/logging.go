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

const DefaultLogLevels = "common.commonspace.headsync=INFO;core.block.editor.spaceview=INFO;*=WARN"
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
	// Stop flushes pending buffered bytes to the underlying writer.
	stopErr := s.BufferedWriteSyncer.Stop()
	closeErr := s.lj.Close()
	return errors.Join(stopErr, closeErr)
}

func newLumberjackSink(u *url.URL) (zap.Sink, error) {
	var lj *lumberjack.Logger
	bufSize := 256 * 1024
	flushInterval := 30 * time.Second
	// On mobile the process can be suspended or killed at any moment, so keep
	// less in-memory to reduce the window of logs lost on abrupt termination.
	if runtime.GOOS == "android" || runtime.GOARCH == "ios" {
		lj = &lumberjack.Logger{
			Filename:   u.Path,
			MaxSize:    10,
			MaxBackups: 2,
			Compress:   false,
		}
		bufSize = 32 * 1024
		flushInterval = 5 * time.Second
	} else {
		lj = &lumberjack.Logger{
			Filename:   u.Path,
			MaxSize:    100,
			MaxBackups: 10,
			Compress:   true,
		}
	}
	return &bufferedLumberjackSink{
		BufferedWriteSyncer: &zapcore.BufferedWriteSyncer{
			WS:            &lumberjackSink{Logger: lj},
			Size:          bufSize,
			FlushInterval: flushInterval,
		},
		lj: lj,
	}, nil
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
