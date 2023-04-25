package logging

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/anytypeio/any-sync/app/logger"
	"github.com/cheggaaa/mb"
	"go.uber.org/zap"
	"gopkg.in/Graylog2/go-gelf.v2/gelf"
)

const graylogHost = "graylog.anytype.io:6888"
const graylogScheme = "gelf+ssl"

var gelfSinkWrapper gelfSink

var defaultCfg = logger.Config{
	Production:   false,
	DefaultLevel: "WARN",
}

func registerGelfSink(config *logger.Config) {
	gelfSinkWrapper.batch = mb.New(1000)
	tlsWriter, err := gelf.NewTLSWriter(graylogHost, nil)
	if err != nil {
		fmt.Printf("failed to init gelf tls: %s", err.Error())
	} else {
		tlsWriter.MaxReconnect = 0
		tlsWriter.ReconnectDelay = time.Second
		gelfSinkWrapper.gelfWriter = tlsWriter
	}

	go gelfSinkWrapper.Run()
	err = zap.RegisterSink(graylogScheme, func(url *url.URL) (zap.Sink, error) {
		// init tlsWriter outside to make sure it is available
		return &gelfSinkWrapper, nil
	})
	config.AddOutputPaths = append(config.AddOutputPaths, graylogScheme+"://")
}

type LWrapper struct {
	zap.SugaredLogger
}

func (l *LWrapper) Warningf(template string, args ...interface{}) {
	l.Warnf(template, args...)
}

func Logger(system string) *LWrapper {
	lg := logger.NewNamed(system)
	return &LWrapper{*(lg.Sugar())}
}

func LoggerNotSugared(system string) *zap.Logger {
	lg := logger.NewNamed(system)

	return lg.Logger
}

func LevelsFromStr(s string) map[string]string {
	levels := make(map[string]string)
	for _, kv := range strings.Split(s, ";") {
		strings.TrimSpace(kv)
		parts := strings.Split(kv, "=")
		var key, value string
		if len(parts) == 1 {
			key = "*"
			value = parts[0]
			levels["*"] = parts[0]
		} else if len(parts) == 2 {
			key = parts[0]
			value = parts[1]
		}
		_, err := zap.ParseAtomicLevel(value)
		if err != nil {
			fmt.Printf("Can't parse log level %s: %s\n", parts[0], err.Error())
			continue
		}
		levels[key] = value
	}
	return levels
}

func init() {
	cfg := defaultCfg
	registerGelfSink(&cfg)
	cfg.NamedLevels = LevelsFromStr(os.Getenv("ANYTYPE_LOG_LEVEL"))
	cfg.ApplyGlobal()

}
