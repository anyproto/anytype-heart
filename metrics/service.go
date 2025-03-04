// Package metrics used to record technical metrics, e.g. app start
package metrics

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/metrics/anymetry"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var (
	Service          = NewService()
	clientMetricsLog = logging.Logger("service-metrics")
	sendInterval     = 30.0 * time.Second
	maxTimeout       = 30 * time.Second
	bufferSize       = 500
	eventsLimit      = 1000 // throttle
)

// First constants must repeat syncstatus.SyncStatus constants for
// avoiding inconsistency with data stored in filestore
const (
	inhouse anymetry.MetricsBackend = iota
)

const inHouseEndpoint = "https://telemetry.anytype.io/2/httpapi"

type SamplableEvent interface {
	anymetry.Event

	Key() string
	Aggregate(other SamplableEvent) SamplableEvent
}

type MetricsService interface {
	InitWithKeys(inHouseKey string)
	SetWorkingDir(workingDir string, accountId string)
	SetAppVersion(path string)
	getWorkingDir() string
	SetStartVersion(v string)
	SetDeviceId(t string)
	SetPlatform(p string)
	SetUserId(id string)
	Send(ev anymetry.Event)
	SendSampled(ev SamplableEvent)
	SetEnabled(isEnabled bool)

	Run()
	Close()
	anymetry.AppInfoProvider
}

type service struct {
	startOnce      *sync.Once
	lock           sync.RWMutex
	appVersion     string
	startVersion   string
	userId         string
	deviceId       string
	platform       string
	workingDir     string
	clients        [1]*client
	alreadyRunning bool
	isEnabled      bool
}

func (s *service) SendSampled(ev SamplableEvent) {
	s.lock.RLock()
	if !s.isEnabled {
		s.lock.RUnlock()
		return
	}
	if ev == nil {
		s.lock.RUnlock()
		return
	}
	backend := s.getBackend(ev.GetBackend())
	s.lock.RUnlock()

	backend.sendSampled(ev)
}

func (s *service) SetEnabled(isEnabled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.isEnabled = isEnabled
}

func NewService() MetricsService {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	return &service{
		startOnce: &sync.Once{},
		clients: [1]*client{
			inhouse: {
				aggregatableMap:  make(map[string]SamplableEvent),
				aggregatableChan: make(chan SamplableEvent, bufferSize),
				ctx:              ctx,
				cancel:           cancel,
			},
		},
	}
}

func (s *service) SetWorkingDir(workingDir string, accountId string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.workingDir = filepath.Join(workingDir, accountId)
}

func (s *service) getWorkingDir() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.workingDir
}

func (s *service) InitWithKeys(inHouseKey string) {
	s.startOnce.Do(func() {
		s.lock.Lock()
		defer s.lock.Unlock()
		s.clients[inhouse].telemetry = anymetry.New(inHouseEndpoint, inHouseKey, true)
	})
}

func (s *service) SetDeviceId(t string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.deviceId = t
}

func (s *service) GetDeviceId() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.deviceId
}

func (s *service) SetPlatform(p string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.platform = p
}

func (s *service) GetPlatform() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.platform
}

func (s *service) SetUserId(id string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.userId = id
}

func (s *service) GetUserId() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.userId
}

func (s *service) SetAppVersion(version string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.appVersion = version
}

func (s *service) GetAppVersion() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.appVersion
}

// SetStartVersion We historically had that field for the version of the service
func (s *service) SetStartVersion(version string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.startVersion = version
}

func (s *service) GetStartVersion() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.startVersion
}

func (s *service) Run() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.isEnabled {
		return
	}
	if s.alreadyRunning {
		return
	}
	s.alreadyRunning = true

	for _, c := range s.clients {
		c.ctx, c.cancel = context.WithCancel(context.Background())
		c.setBatcher(mb.New[anymetry.Event](eventsLimit))
		go c.startAggregating()
		go c.startSendingBatchMessages(s)
	}
}

func (s *service) Close() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.isEnabled {
		return
	}
	for _, c := range s.clients {
		c.Close()
	}
	s.alreadyRunning = false
}

func (s *service) Send(ev anymetry.Event) {
	s.lock.RLock()
	if !s.isEnabled {
		s.lock.RUnlock()
		return
	}
	if ev == nil {
		s.lock.RUnlock()
		return
	}
	backend := s.getBackend(ev.GetBackend())
	s.lock.RUnlock()

	backend.send(ev)
}

func (s *service) getBackend(backend anymetry.MetricsBackend) *client {
	switch backend {
	case inhouse:
		return s.clients[inhouse]
	}
	return nil
}
