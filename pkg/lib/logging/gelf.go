package logging

import (
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/cheggaaa/mb/v3"
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

// gelfEntry is a lightweight queue item — just the log bytes and timestamp.
// The full gelf.Message is constructed in the consumer goroutine to avoid
// copying a large struct through the mb queue on every log line.
type gelfEntry struct {
	data     []byte
	timeUnix float64
}

var gelfEntryPool = sync.Pool{
	New: func() interface{} {
		return &gelfEntry{data: make([]byte, 0, 512)}
	},
}

func registerGelfSink(config *logger.Config) {
	gelfSinkWrapper.rawExtraDirty = true
	gelfSinkWrapper.batch = mb.New[*gelfEntry](1000)
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
	batch         *mb.MB[*gelfEntry]
	gelfWriter    gelf.Writer
	version       string
	account       string
	host          string
	lastErrorAt   time.Time
	rawExtraBuf   []byte // pre-serialized JSON for Extra fields
	rawExtraDirty bool
}

func (gs *gelfSink) Run() {
	for {
		if !gs.lastErrorAt.IsZero() && gs.lastErrorAt.Add(logWriteDiscardThreshold).After(time.Now()) {
			// do not try to reconnect to aggressively in case of error
			// it's ok if we will lost some of msgs on shutdown because of it
			time.Sleep(time.Second * 5)
			continue
		}

		entries, err := gs.batch.NewCond().WithMax(1).Wait(context.Background())
		if err != nil {
			return
		}
		if len(entries) == 0 {
			return
		}

		gs.RLock()
		host := gs.host
		rawExtra := gs.rawExtraBuf
		gs.RUnlock()

		for _, entry := range entries {
			msg := gelf.Message{
				Version:  "1.1",
				Host:     host,
				Short:    string(entry.data),
				TimeUnix: entry.timeUnix,
				Level:    0,
				RawExtra: rawExtra,
			}
			// Return entry to pool
			entry.data = entry.data[:0]
			gelfEntryPool.Put(entry)

			err := gs.gelfWriter.WriteMessage(&msg)
			if err != nil {
				if gs.lastErrorAt.IsZero() || gs.lastErrorAt.Add(printErrorThreshold).Before(time.Now()) {
					fmt.Fprintf(os.Stderr, "failed to write to gelf: %v\n", err)
				}
				gs.lastErrorAt = time.Now()
				// Don't re-enqueue on error — the entry is already returned to pool
			}
		}
	}
}

// rebuildRawExtra pre-serializes the Extra JSON so Write doesn't allocate a map per call.
// Must be called with gs.Lock held.
func (gs *gelfSink) rebuildRawExtra() {
	if !gs.rawExtraDirty {
		return
	}
	gs.rawExtraBuf, _ = json.Marshal(map[string]interface{}{
		"_mwver":   gs.version,
		"_account": gs.account,
	})
	gs.rawExtraDirty = false
}

func (gs *gelfSink) Write(b []byte) (int, error) {
	gs.Lock()
	defer gs.Unlock()
	if gs.gelfWriter == nil {
		return 0, fmt.Errorf("gelfWriter is nil")
	}

	gs.rebuildRawExtra()

	entry := gelfEntryPool.Get().(*gelfEntry)
	entry.data = append(entry.data, b...)
	entry.timeUnix = float64(time.Now().UnixNano()) / float64(time.Second)

	err := gs.batch.TryAdd(entry)
	if errors.Is(err, mb.ErrOverflowed) {
		// batch is overflowed, probably machine has some internet problems
		entry.data = entry.data[:0]
		gelfEntryPool.Put(entry)
		loggerGraylogMBSkipped.Add(1)
		return len(b), nil
	} else if err != nil {
		entry.data = entry.data[:0]
		gelfEntryPool.Put(entry)
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
	gs.rawExtraDirty = true
}

func (gs *gelfSink) SetAccount(account string) {
	gs.Lock()
	defer gs.Unlock()
	gs.account = account
	gs.rawExtraDirty = true
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
