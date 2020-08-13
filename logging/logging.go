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
var gelfSinkWrapper *gelfSink
var m = sync.Mutex{}

var defaultCfg = logging.Config{
	Format: logging.JSONOutput,
	Level:  logging.LevelDebug,
	Stderr: false,
	Stdout: true,
	URL:    graylogScheme + "://" + graylogHost,
}

type gelfSink struct {
	sync.RWMutex
	gelfWriter gelf.Writer

	version string
	host    string
}

func (gs *gelfSink) Write(b []byte) (int, error) {
	gs.RLock()
	defer gs.RUnlock()

	msg := gelf.Message{
		Version:  gs.version,
		Host:     gs.host,
		Short:    "",
		Full:     string(b),
		TimeUnix: 0,
		Level:    0,
		Facility: "",
	}

	return len(b), gs.gelfWriter.WriteMessage(&msg)
}

func (gs *gelfSink) Close() error {
	return gs.gelfWriter.Close()
}

func (gs *gelfSink) Sync() error {
	return nil
}

func (gs *gelfSink) SetHost(host string) {
	gs.Lock()
	defer gs.Unlock()
	gs.host = host
}

func (gs *gelfSink) SetVersion(version string) {
	gs.Lock()
	defer gs.Unlock()
	gs.version = version
}

func init() {
	var err error
	tlsWriter, err := gelf.NewTLSWriter(graylogHost, nil)
	if err != nil {
		log.Error(err)
		return
	}
	gelfSinkWrapper.gelfWriter = tlsWriter

	err = zap.RegisterSink(graylogScheme, func(url *url.URL) (zap.Sink, error) {
		// init tlsWriter outside to make sure it is available
		return gelfSinkWrapper, nil
	})

	if err != nil {
		log.Error("failed to register zap sink", err.Error())
	}

	logging.SetupLogging(defaultCfg)
}

func Logger(system string) zap.SugaredLogger {
	logger := logging.Logger(system)

	return logger.SugaredLogger
}

func SetLoggingFilepath(logPath string) {
	cfg := defaultCfg

	cfg.Format = logging.PlaintextOutput
	cfg.File = filepath.Join(logPath, "anytype.log")

	logging.SetupLogging(defaultCfg)
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
