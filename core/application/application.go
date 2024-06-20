package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/trace"
	"sync"

	"github.com/anyproto/any-sync/app"
	exptrace "golang.org/x/exp/trace"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-core-account")

type Service struct {
	lock sync.RWMutex

	app *app.App

	mnemonic string

	// memoized private key derived from mnemonic, used for signing session tokens
	sessionSigningKey []byte

	rootPath          string
	clientWithVersion string
	eventSender       event.Sender
	sessions          session.Service

	traceRecorder     *exptrace.FlightRecorder
	traceRecorderLock sync.Mutex

	appAccountStartInProcessCancel      context.CancelFunc
	appAccountStartInProcessCancelMutex sync.Mutex
}

func New() *Service {
	return &Service{
		sessions: session.New(),
	}
}

func (s *Service) GetApp() *app.App {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.app
}

func (s *Service) requireClientWithVersion() {
	if s.clientWithVersion == "" {
		panic(errors.New("client platform with the version must be set using the MetricsSetParameters method"))
	}
}

func (s *Service) Stop() error {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.stop()
}

func (s *Service) stop() error {
	ctx, task := trace.NewTask(context.Background(), "application.stop")
	defer task.End()

	if s != nil && s.app != nil {
		err := s.app.Close(ctx)
		if err != nil {
			log.Warnf("error while stop anytype: %v", err)
		}

		s.app = nil
	}
	return nil
}

func (s *Service) SetClientVersion(platform string, version string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.clientWithVersion = platform + ":" + version
}

func (s *Service) SetEventSender(sender event.Sender) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.eventSender = sender
}

func (s *Service) GetEventSender() event.Sender {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.eventSender
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
