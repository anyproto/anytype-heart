package logging

import (
	"errors"
	"expvar"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/cheggaaa/mb"
	"go.uber.org/zap"
	"gopkg.in/Graylog2/go-gelf.v2/gelf"
)

const (
	printErrorThreshold      = time.Minute
	logWriteDiscardThreshold = time.Second * 30
	graylogHost              = "graylog.anytype.io:6888"
	graylogScheme            = "gelf+ssl"
)

var gelfSinkWrapper gelfSink

var (
	loggerGraylogMBSkipped = expvar.NewInt("logger_graylog_mb_skipped")
)

func registerGelfSink(config *logger.Config) {
	gelfSinkWrapper.batch = mb.New(1000)
	tlsWriter, err := gelf.NewTLSWriter(graylogHost, nil)
	if err != nil {
		fmt.Printf("failed to init gelf tls: %s", err)
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

type gelfSink struct {
	sync.RWMutex
	batch       *mb.MB
	gelfWriter  gelf.Writer
	version     string
	account     string
	host        string
	lastErrorAt time.Time
}

func (gs *gelfSink) Run() {
	for {
		if !gs.lastErrorAt.IsZero() && gs.lastErrorAt.Add(logWriteDiscardThreshold).After(time.Now()) {
			// do not try to reconnect to aggressively in case of error
			// it's ok if we will lost some of msgs on shutdown because of it
			time.Sleep(time.Second * 5)
			continue
		}

		msgs := gs.batch.WaitMax(1)
		if len(msgs) == 0 {
			return
		}

		for _, msg := range msgs {
			msgCasted, ok := msg.(gelf.Message)
			if !ok {
				continue
			}
			err := gs.gelfWriter.WriteMessage(&msgCasted)
			if err != nil {
				if gs.lastErrorAt.IsZero() || gs.lastErrorAt.Add(printErrorThreshold).Before(time.Now()) {
					fmt.Fprintf(os.Stderr, "failed to write to gelf: %v\n", err)
				}
				gs.lastErrorAt = time.Now()
				_ = gs.batch.TryAdd(msg)
				// batch can be overflowed, let's do our best and ignore errors
			}
		}
	}
}

func (gs *gelfSink) Write(b []byte) (int, error) {
	gs.Lock()
	defer gs.Unlock()
	if gs.gelfWriter == nil {
		return 0, fmt.Errorf("gelfWriter is nil")
	}

	msg := gelf.Message{
		Version:  "1.1",
		Host:     gs.host,
		Short:    string(b),
		TimeUnix: float64(time.Now().UnixNano()) / float64(time.Second),
		Level:    0,
		Extra:    map[string]interface{}{"_mwver": gs.version, "_account": gs.account},
	}

	err := gs.batch.TryAdd(msg)
	if errors.Is(err, mb.ErrOverflowed) {
		// batch is overflowed, probably machine has some internet problems
		// we don't want to spam with mb overflowed errors, so let's just ignore it and return as success
		loggerGraylogMBSkipped.Add(1)
		return len(b), nil
	} else if err != nil {
		return 0, err
	}

	return len(b), nil
}

func (gs *gelfSink) Close() error {
	gs.Lock()
	defer gs.Unlock()
	err := gs.batch.Close()
	if err != nil {
		return err
	}
	if skipped := loggerGraylogMBSkipped.Value(); skipped > 0 {
		fmt.Fprintf(os.Stderr, "gelf: skipped %d messages\n", skipped)
	}
	return gs.gelfWriter.Close()
}

func (gs *gelfSink) Sync() error {
	// todo: should we use Sync to flush batch?
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

func (gs *gelfSink) SetAccount(account string) {
	gs.Lock()
	defer gs.Unlock()
	gs.account = account
}

func SetVersion(version string) {
	gelfSinkWrapper.SetVersion(version)
}

func SetHost(host string) {
	gelfSinkWrapper.SetHost(host)
}

func SetAccount(account string) {
	gelfSinkWrapper.SetAccount(account)
}
