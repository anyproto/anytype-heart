package logging

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gobwas/glob"
	logging "github.com/ipfs/go-log/v2"
	"go.uber.org/zap"
	"gopkg.in/Graylog2/go-gelf.v2/gelf"
)

const graylogHost = "graylog.anytype.io:6888"
const graylogScheme = "gelf+ssl"

var log = logging.Logger("anytype-logger")

var DefaultLogLevel = logging.LevelError
var logLevelsStr string
var gelfSinkWrapper gelfSink
var m = sync.Mutex{}

var defaultCfg = logging.Config{
	Format: logging.JSONOutput,
	Level:  logging.LevelDebug,
	Stderr: false,
	Stdout: true,
	URL:    graylogScheme + "://" + graylogHost,
}

func getLoggingConfig() logging.Config {
	cfg := defaultCfg

	if os.Getenv("ANYTYPE_LOG_NOGELF") == "1" {
		// set default format to colored text
		cfg.Format = logging.ColorizedOutput
		cfg.URL = ""
	}

	var format string
	if v := os.Getenv("ANYTYPE_LOG_FMT"); v != "" {
		format = v
	} else if v := os.Getenv("GO_LOG_FMT"); v != "" {
		// support native ipfs logger env var
		format = v
	}

	switch format {
	case "color":
		cfg.Format = logging.ColorizedOutput
	case "nocolor":
		cfg.Format = logging.PlaintextOutput
	case "json":
		cfg.Format = logging.JSONOutput
	}

	return cfg
}

func init() {
	tlsWriter, err := gelf.NewTLSWriter(graylogHost, nil)
	if err != nil {
		log.Error(err)
		return
	}
	gelfSinkWrapper.gelfWriter = tlsWriter

	err = zap.RegisterSink(graylogScheme, func(url *url.URL) (zap.Sink, error) {
		// init tlsWriter outside to make sure it is available
		return &gelfSinkWrapper, nil
	})

	if err != nil {
		log.Error("failed to register zap sink", err.Error())
	}

	cfg := getLoggingConfig()
	logging.SetupLogging(cfg)
}

func Logger(system string) zap.SugaredLogger {
	logger := logging.Logger(system)

	return logger.SugaredLogger
}

func SetLoggingFilepath(logPath string) {
	cfg := defaultCfg

	cfg.Format = logging.PlaintextOutput
	cfg.File = filepath.Join(logPath, "anytype.log")

	logging.SetupLogging(cfg)
}

func SetLoggingFormat(format logging.LogFormat) {
	cfg := getLoggingConfig()
	cfg.Format = format

	logging.SetupLogging(cfg)
}

func ApplyLevels(str string) {
	logLevelsStr = str
	setSubsystemLevels()
}

func ApplyLevelsFromEnv() {
	ApplyLevels(os.Getenv("ANYTYPE_LOG_LEVEL"))
}

func setSubsystemLevels() {
	m.Lock()
	defer m.Unlock()
	logLevels := make(map[string]string)
	if logLevelsStr != "" {
		for _, level := range strings.Split(logLevelsStr, ";") {
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
			if err != logging.ErrNoSuchLogger {
				// it returns ErrNoSuchLogger when we don't initialised this subsystem yet
				log.Errorf("subsystem %s has incorrect log level '%s': %w", subsystem, level, err)
			}
		}
	}
}

func SetVersion(version string) {
	gelfSinkWrapper.SetVersion(version)
}

func SetHost(host string) {
	gelfSinkWrapper.SetHost(host)
}
