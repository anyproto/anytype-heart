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
	"time"

	exptrace "golang.org/x/exp/trace"

	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *Service) RunProfiler(ctx context.Context, seconds int) (string, error) {
	// Start
	inFlightTraceBuf, err := s.stopAndGetInFlightTrace()
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

func (s *Service) stopAndGetInFlightTrace() (*bytes.Buffer, error) {
	s.traceRecorderLock.Lock()
	defer s.traceRecorderLock.Unlock()

	if s.traceRecorder != nil {
		buf := bytes.NewBuffer(nil)
		_, err := s.traceRecorder.WriteTo(buf)
		if err != nil {
			s.traceRecorderLock.Unlock()
			return nil, fmt.Errorf("write in-flight trace: %w", err)
		}
		s.traceRecorder.Stop()
		s.traceRecorder = nil
		return buf, nil
	}
	return nil, nil
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
	s.traceRecorderLock.Lock()
	defer s.traceRecorderLock.Unlock()

	if s.traceRecorder == nil {
		return "", errors.New("no running trace recorder")
	}

	buf := bytes.NewBuffer(nil)
	_, err := s.traceRecorder.WriteTo(buf)
	if err != nil {
		return "", fmt.Errorf("write trace: %w", err)
	}

	f, err := os.CreateTemp("", "login-trace-*.trace")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	_, err = io.Copy(f, buf)
	if err != nil {
		return "", errors.Join(f.Close(), fmt.Errorf("copy trace: %w", err))
	}
	return f.Name(), f.Close()
}

func (s *Service) stopTraceRecorder() {
	s.traceRecorderLock.Lock()
	if s.traceRecorder != nil {
		s.traceRecorder.Stop()
		s.traceRecorder = nil
	}
	s.traceRecorderLock.Unlock()
}

func (s *Service) startTraceRecorder() {
	flightRecorder := exptrace.NewFlightRecorder()
	flightRecorder.SetPeriod(60 * time.Second)
	err := flightRecorder.Start()
	if err == nil {
		s.traceRecorderLock.Lock()
		s.traceRecorder = flightRecorder
		s.traceRecorderLock.Unlock()
	}
}
