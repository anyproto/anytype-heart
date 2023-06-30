package logging

import (
	"fmt"
	"os"
	"strings"

	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/util/vcs"
)

var defaultCfg = logger.Config{
	Production:   false,
	DefaultLevel: "WARN",
	Format:       logger.JSONOutput,
}

type LWrapper struct {
	*zap.SugaredLogger
}

func (l LWrapper) Warningf(template string, args ...interface{}) {
	l.Warnf(template, args...)
}

func Logger(system string) *zap.SugaredLogger {
	return logger.NewNamedSugared(system)
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
			fmt.Printf("Can't parse log level %s: %s\n", parts[0], err.Error())
			continue
		}
		levels = append(levels, logger.NamedLevel{Name: key, Level: value})
	}
	return levels
}

func init() {
	cfg := defaultCfg
	info := vcs.GetVCSInfo()

	SetVersion(info.Version())

	if os.Getenv("ANYTYPE_LOG_NOGELF") == "1" {
		cfg.Format = logger.ColorizedOutput
	} else {
		registerGelfSink(&cfg)
	}
	cfg.Levels = LevelsFromStr(os.Getenv("ANYTYPE_LOG_LEVEL"))
	cfg.ApplyGlobal()
}
