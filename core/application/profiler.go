package application

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"

	exptrace "golang.org/x/exp/trace"

	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *Service) RunProfiler(ctx context.Context, seconds int) (string, error) {
	// Start
	inFlightTraceBuf, err := s.traceRecorder.stopAndGetInFlightTrace()
	if err != nil {
		return "", err
	}

	var tracerBuf bytes.Buffer
	err = trace.Start(&tracerBuf)
	if err != nil {
		return "", fmt.Errorf("start tracer: %w", err)
	}

	var cpuProfileBuf bytes.Buffer
	err = pprof.StartCPUProfile(&cpuProfileBuf)
	if err != nil {
		return "", fmt.Errorf("start cpu profile: %w", err)
	}

	var heapStartBuf bytes.Buffer
	err = pprof.WriteHeapProfile(&heapStartBuf)
	if err != nil {
		return "", fmt.Errorf("write starting heap profile: %w", err)
	}
	goroutinesStart := debug.Stack(true)

	// Wait
	select {
	case <-time.After(time.Duration(seconds) * time.Second):
	case <-ctx.Done():
	}

	// End
	pprof.StopCPUProfile()
	trace.Stop()
	var heapEndBuf bytes.Buffer
	err = pprof.WriteHeapProfile(&heapEndBuf)
	if err != nil {
		return "", fmt.Errorf("write ending heap profile: %w", err)
	}
	goroutinesEnd := debug.Stack(true)

	// Write
	f, err := os.CreateTemp("", "anytype_profile.*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	files := []zipFile{
		{name: "trace", data: &tracerBuf},
		{name: "cpu_profile", data: &cpuProfileBuf},
		{name: "heap_start", data: &heapStartBuf},
		{name: "heap_end", data: &heapEndBuf},
		{name: "goroutines_start.txt", data: bytes.NewReader(goroutinesStart)},
		{name: "goroutines_end.txt", data: bytes.NewReader(goroutinesEnd)},
	}
	if inFlightTraceBuf != nil {
		files = append(files, zipFile{name: "in_flight_trace", data: inFlightTraceBuf})
	}
	err = createZipArchive(f, files)
	if err != nil {
		return "", errors.Join(fmt.Errorf("create zip archive: %w", err), f.Close())
	}
	return f.Name(), f.Close()
}

type zipFile struct {
	name string
	data io.Reader
}

func createZipArchive(w io.Writer, files []zipFile) error {
	zipw := zip.NewWriter(w)
	err := func() error {
		for _, file := range files {
			f, err := zipw.Create(file.name)
			if err != nil {
				return fmt.Errorf("create file in zip archive: %w", err)
			}
			_, err = io.Copy(f, file.data)
			if err != nil {
				return fmt.Errorf("copy data to file: %w", err)
			}
		}
		return nil
	}()
	return errors.Join(err, zipw.Close())
}

func (s *Service) SaveLoginTrace() (string, error) {
	return s.traceRecorder.save()
}

// traceRecorder is a helper to start and stop flight trace recorder
type traceRecorder struct {
	lock            sync.Mutex
	recorder        *exptrace.FlightRecorder
	lastRecordedBuf *bytes.Buffer
}

func (r *traceRecorder) save() (string, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	var traceReader io.Reader
	if r.recorder == nil {
		if r.lastRecordedBuf == nil {
			return "", errors.New("no running trace recorder")
		}
		traceReader = r.lastRecordedBuf
		r.lastRecordedBuf = nil
	} else {
		buf := bytes.NewBuffer(nil)
		_, err := r.recorder.WriteTo(buf)
		if err != nil {
			return "", fmt.Errorf("write trace: %w", err)
		}
		traceReader = buf
	}

	f, err := os.CreateTemp("", "login-trace-*.trace")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	_, err = io.Copy(f, traceReader)
	if err != nil {
		return "", errors.Join(f.Close(), fmt.Errorf("copy trace: %w", err))
	}
	return f.Name(), f.Close()
}

func (r *traceRecorder) start() {
	flightRecorder := exptrace.NewFlightRecorder()
	flightRecorder.SetPeriod(60 * time.Second)
	err := flightRecorder.Start()
	if err == nil {
		r.lock.Lock()
		r.recorder = flightRecorder
		r.lock.Unlock()
	}
}

func (r *traceRecorder) stop() {
	r.lock.Lock()
	if r.recorder != nil {
		r.lastRecordedBuf = bytes.NewBuffer(nil)
		_, err := r.recorder.WriteTo(r.lastRecordedBuf)
		if err != nil {
			log.With("error", err).Error("save recorded trace to buf")
		}
		err = r.recorder.Stop()
		if err != nil {
			log.With("error", err).Error("stop trace recorder")
		}
		r.recorder = nil
	}
	r.lock.Unlock()
}

func (r *traceRecorder) stopAndGetInFlightTrace() (*bytes.Buffer, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.recorder != nil {
		buf := bytes.NewBuffer(nil)
		_, err := r.recorder.WriteTo(buf)
		if err != nil {
			return nil, fmt.Errorf("write in-flight trace: %w", err)
		}
		err = r.recorder.Stop()
		if err != nil {
			log.With("error", err).Error("stop trace recorder")
		}
		r.recorder = nil
		return buf, nil
	}
	return nil, nil
}
