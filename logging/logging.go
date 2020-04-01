package logging

import (
	"os"
	"strings"
	"sync"

	"github.com/gobwas/glob"
	logging "github.com/ipfs/go-log"
	log2 "github.com/ipfs/go-log/v2"
)

var log = logging.Logger("anytype-logger")

var DefaultLogLevel = logging.LevelError
var m = sync.Mutex{}

func Logger(system string) *logging.ZapEventLogger {
	logger := logging.Logger(system)
	ApplyLevelsFromEnv()

	return logger
}

func ApplyLevelsFromEnv() {
	m.Lock()
	defer m.Unlock()
	levels := os.Getenv("ANYTYPE_LOG_LEVEL")
	logLevels := make(map[string]string)
	if levels != "" {
		for _, level := range strings.Split(levels, ";") {
			parts := strings.Split(level, "=")
			var subsystemPattern glob.Glob
			var level string
			if len(parts) == 1 {
				subsystemPattern = glob.MustCompile("anytype-*")
				level = parts[0]
			} else if len(parts) == 2 {
				var err error
				subsystemPattern, err = glob.Compile(parts[0])
				if err != nil {
					log.Errorf("failed to parse glob pattern '%s': %w", parts[1], err)
					continue
				}
				level = parts[1]
			}

			for _, subsystem := range logging.GetSubsystems() {
				if subsystemPattern.Match(subsystem) {
					logLevels[subsystem] = level
				}
			}
		}
	}

	if len(logLevels) == 0 {
		logging.SetAllLoggers(DefaultLogLevel)
		return
	}

	for subsystem, level := range logLevels {
		err := logging.SetLogLevel(subsystem, level)
		if err != nil {
			if err != log2.ErrNoSuchLogger {
				// it returns ErrNoSuchLogger when we don't initialised this subsystem yet
				log.Errorf("subsystem %s has incorrect log level '%s': %w", subsystem, level, err)
			}
		}
	}
}
