package logging

import (
	"fmt"
	"os"
	"strings"

	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/util/vcs"
)

const DefaultLogLevels = "common.commonspace.headsync=INFO;core.block.editor.spaceview=INFO;*=WARN"

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

func init() {
	if os.Getenv("ANYTYPE_LOG_NOGELF") == "1" {
		DefaultCfg.Format = logger.ColorizedOutput
	} else {
		registerGelfSink(&DefaultCfg)
		info := vcs.GetVCSInfo()
		SetVersion(info.Version())
	}
	logLevels := os.Getenv("ANYTYPE_LOG_LEVEL")
	if logLevels == "" {
		logLevels = DefaultLogLevels
	}

	SetLogLevels(logLevels)
}
