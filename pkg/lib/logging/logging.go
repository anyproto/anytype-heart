package logging

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/anyproto/anytype-heart/pkg/lib/environment"
	"github.com/anyproto/anytype-heart/util/vcs"
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

func (s *lumberjackSink) Sync() error {
	return nil
}

func newLumberjackSink(u *url.URL) (zap.Sink, error) {
	// if android, limit the log file size to 10MB
	if runtime.GOOS == "android" || runtime.GOARCH == "ios" {
		return &lumberjackSink{
			Logger: &lumberjack.Logger{
				Filename:   u.Path,
				MaxSize:    10,
				MaxBackups: 2,
				Compress:   false,
			},
		}, nil
	}
	return &lumberjackSink{
		Logger: &lumberjack.Logger{
			Filename:   u.Path,
			MaxSize:    100,
			MaxBackups: 10,
			Compress:   true,
		},
	}, nil
}

func Init(root string, logLevels string, sendLogs bool, saveLogs bool) {
	if root != "" {
		environment.ROOT_PATH = filepath.Join(root, "common")
		err := os.MkdirAll(environment.ROOT_PATH, 0755)
		if err != nil {
			if !os.IsExist(err) {
				fmt.Println("failed to create global dir", err)
			}
		}
	}

	if os.Getenv("ANYTYPE_LOG_NOGELF") == "1" || !sendLogs {
		if !saveLogs {
			DefaultCfg.Format = logger.ColorizedOutput
		}
	} else {
		registerGelfSink(&DefaultCfg)
		info := vcs.GetVCSInfo()
		SetVersion(info.Version())
	}
	if saveLogs {
		registerLumberjackSink(environment.ROOT_PATH, &DefaultCfg)
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

func registerLumberjackSink(globalRoot string, config *logger.Config) {
	if globalRoot == "" {
		fmt.Println("globalRoot dir is not set")
		return
	}
	logsDir := filepath.Join(globalRoot, "logs")
	err := os.Mkdir(logsDir, 0755)
	if err != nil && !os.IsExist(err) {
		fmt.Println("failed to create logs dir", err)
		// do not continue if logs dir can't be created
		return
	}

	err = zap.RegisterSink(lumberjackScheme, newLumberjackSink)
	if err != nil {
		fmt.Println("failed to register lumberjack sink", err)
	}

	environment.LOG_PATH = filepath.Join(logsDir, "anytype.log")
	config.AddOutputPaths = append(config.AddOutputPaths, lumberjackScheme+":"+environment.LOG_PATH)
}
