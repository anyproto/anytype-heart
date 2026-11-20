package anymetry

import "github.com/valyala/fastjson"

type Service interface {
	SendEvents(amplEvents []Event, info AppInfoProvider) error
}

type AppInfoProvider interface {
	GetAppVersion() string
	GetStartVersion() string
	GetDeviceId() string
	GetPlatform() string
	GetUserId() string
}

type Event interface {
	GetBackend() MetricsBackend
	MarshalFastJson(arena *fastjson.Arena) JsonEvent
	SetTimestamp()
	GetTimestamp() int64
}

type MetricsBackend int
type JsonEvent *fastjson.Value

type noOp struct{}

func (noOp) SendEvents([]Event, AppInfoProvider) error { return nil }

func New(eventEndpoint string, key string, isCompressed bool) Service {
	return noOp{}
}
