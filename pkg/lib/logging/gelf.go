package logging

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cheggaaa/mb"
	"gopkg.in/Graylog2/go-gelf.v2/gelf"
)

const (
	printErrorThreshold      = time.Minute
	logWriteDiscardThreshold = time.Second * 30
)

type gelfSink struct {
	sync.RWMutex
	batch       *mb.MB
	gelfWriter  gelf.Writer
	version     string
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
					fmt.Fprintf(os.Stderr, "failed to write to gelf: %v\n", err.Error())
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
		Extra:    map[string]interface{}{"_mwver": gs.version},
	}

	err := gs.batch.TryAdd(msg)
	if err != nil {
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
