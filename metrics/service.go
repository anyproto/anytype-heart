// Package metrics used to record technical metrics, e.g. app start
package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/metrics/amplitude"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var (
	Service          = NewService()
	clientMetricsLog = logging.Logger("service-metrics")
	sendInterval     = 10.0 * time.Second
	maxTimeout       = 30 * time.Second
	bufferSize       = 500
)

type MetricsBackend int

// First constants must repeat syncstatus.SyncStatus constants for
// avoiding inconsistency with data stored in filestore
const (
	ampl MetricsBackend = iota
	inhouse
)

const amplEndpoint = "https://amplitude.anytype.io/2/httpapi"
const inhouseEndpoint = "https://inhouse.anytype.io/2/httpapi" //todo change to inhouse

type Event interface {
	getBackend() MetricsBackend
	get() *anyEvent
}

type SamplableEvent interface {
	Event

	Key() string
	Aggregate(other SamplableEvent) SamplableEvent
}

type anyEvent struct {
	eventType string
	eventData map[string]interface{}
}

type appInfoProvider interface {
	getAppVersion() string
	getStartVersion() string
	getDeviceId() string
	getPlatform() string
	getUserId() string
}

type MetricsService interface {
	InitAmplWithKey(k string)
	SetAppVersion(v string)
	SetStartVersion(v string)
	SetDeviceId(t string)
	SetPlatform(p string)
	SetUserId(id string)
	Send(ev Event)
	SendSampled(ev SamplableEvent)

	Run()
	Close()
	appInfoProvider
}

type service struct {
	lock         sync.RWMutex
	appVersion   string
	startVersion string
	userId       string
	deviceId     string
	platform     string
	clients      map[MetricsBackend]*client
}

func (s *service) SendSampled(ev SamplableEvent) {
	if ev == nil {
		return
	}
	s.getBackend(ev.getBackend()).sendSampled(ev)
}

func NewService() MetricsService {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	return &service{
		clients: map[MetricsBackend]*client{
			ampl: {
				aggregatableMap:  make(map[string]SamplableEvent),
				aggregatableChan: make(chan SamplableEvent, bufferSize),
				ctx:              ctx,
				cancel:           cancel,
			},
			inhouse: {
				aggregatableMap:  make(map[string]SamplableEvent),
				aggregatableChan: make(chan SamplableEvent, bufferSize),
				ctx:              ctx,
				cancel:           cancel,
				amplitude:        amplitude.New(inhouseEndpoint, ""),
			},
		},
	}
}

func (s *service) InitAmplWithKey(key string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.clients[ampl].amplitude = amplitude.New(amplEndpoint, key)
}

func (s *service) SetDeviceId(t string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.deviceId = t
}

func (s *service) getDeviceId() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.deviceId
}

func (s *service) SetPlatform(p string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.platform = p
}

func (s *service) getPlatform() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.platform
}

func (s *service) SetUserId(id string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.userId = id
}

func (s *service) getUserId() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.userId
}

func (s *service) SetAppVersion(version string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.appVersion = version
}

func (s *service) getAppVersion() string {
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

func (s *service) getStartVersion() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.startVersion
}

func (s *service) Run() {
	s.lock.Lock()
	defer s.lock.Unlock()
	for _, c := range s.clients {
		c.ctx, c.cancel = context.WithCancel(context.Background())
		c.batcher = mb.New[amplitude.Event](0)
		c.closeChannel = make(chan struct{})
		go c.startAggregating(s)
		go c.startSendingBatchMessages(s)
	}
}

func (s *service) Close() {
	s.lock.Lock()
	for _, c := range s.clients {
		c.Close()
	}
	defer s.lock.Unlock()
}

func (s *service) Send(ev Event) {
	if ev == nil {
		return
	}
	s.getBackend(ev.getBackend()).send(s, ev)
}

func (s *service) getBackend(backend MetricsBackend) *client {
	s.lock.RLock()
	defer s.lock.RUnlock()

	switch backend {
	case ampl:
		amplClient := s.clients[ampl]
		return amplClient
	case inhouse:
		return s.clients[inhouse]
	}
	return nil
}
