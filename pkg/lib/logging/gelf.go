package logging

import (
	"fmt"
	"sync"
	"time"

	"gopkg.in/Graylog2/go-gelf.v2/gelf"
)

type gelfSink struct {
	sync.RWMutex
	gelfWriter gelf.Writer

	version string
	host    string
}

func (gs *gelfSink) Write(b []byte) (int, error) {
	gs.RLock()
	defer gs.RUnlock()
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

	go func() {
		// we want to make sure we don't waiting for the network when printing logs
		// @todo: need to be buffered sending
		err := gs.gelfWriter.WriteMessage(&msg)
		if err != nil {
			fmt.Printf("failed to write to gelf: %s\n", err.Error())
		}
	}()

	return len(b), nil
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
