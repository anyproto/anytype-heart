package logging

import (
	"os"
	"strings"
	"sync"

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
			if len(parts) == 1 {
				// set all anytype-* subsystems when have a simple level like:
				// ANYTYPE_LOG_LEVEL=DEBUG
				for _, subsystem := range logging.GetSubsystems() {
					if strings.HasPrefix(subsystem, "anytype-") {
						logLevels[subsystem] = parts[0]
					}
				}
			} else if len(parts) == 2 {
				logLevels[parts[0]] = parts[1]
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
