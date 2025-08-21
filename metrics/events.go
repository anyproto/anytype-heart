package metrics

import (
	"fmt"
	"time"

	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/metrics/anymetry"
)

const (
	CtxKeyEntrypoint = "entrypoint"
	CtxKeyRPC        = "rpc"
)

type ReindexType int

const (
	ReindexTypeThreads ReindexType = iota
	ReindexTypeFiles
	ReindexTypeBundledRelations
	ReindexTypeBundledTypes
	ReindexTypeBundledObjects
	ReindexTypeBundledTemplates
	ReindexTypeOutdatedHeads
	ReindexTypeSystem
)

func (t ReindexType) String() string {
	switch t {
	case ReindexTypeThreads:
		return "threads"
	case ReindexTypeFiles:
		return "files"
	case ReindexTypeBundledRelations:
		return "bundled_relations"
	case ReindexTypeBundledTypes:
		return "bundled_types"
	case ReindexTypeBundledObjects:
		return "bundled_objects"
	case ReindexTypeBundledTemplates:
		return "bundled_templates"
	case ReindexTypeOutdatedHeads:
		return "outdated_heads"
	case ReindexTypeSystem:
		return "system"
	}
	return "unknown"
}

func setupProperties(arena *fastjson.Arena, eventType string) (*fastjson.Value, *fastjson.Value) {
	event := arena.NewObject()
	properties := arena.NewObject()
	event.Set("event_type", arena.NewString(eventType))
	event.Set("event_properties", properties)
	return event, properties
}

type ImportStartedEvent struct {
	baseInfo
	ID         string
	ImportType string
}

func (i *ImportStartedEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (i *ImportStartedEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "import_started")

	properties.Set("import_id", arena.NewString(i.ID))
	properties.Set("import_type", arena.NewString(i.ImportType))

	return event
}

type ImportFinishedEvent struct {
	baseInfo
	ID         string
	ImportType string
}

func (i *ImportFinishedEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (i *ImportFinishedEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "import_finished")

	properties.Set("import_id", arena.NewString(i.ID))
	properties.Set("import_type", arena.NewString(i.ImportType))

	return event
}

type baseInfo struct {
	time int64
}

func (b *baseInfo) SetTimestamp() {
	b.time = time.Now().UnixMilli()
}

func (b *baseInfo) GetTimestamp() int64 {
	return b.time
}

type MethodEvent struct {
	baseInfo
	methodName  string
	middleTime  int64
	errorCode   int64
	description string
}

func (c *MethodEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *MethodEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "MethodEvent")

	properties.Set("methodName", arena.NewString(c.methodName))
	properties.Set("middleTime", arena.NewNumberInt(int(c.middleTime)))
	properties.Set("errorCode", arena.NewNumberInt(int(c.errorCode)))
	properties.Set("description", arena.NewString(c.description))
	return event
}

type ChangeEvent struct {
	baseInfo
	ChangeName string
	SbType     string
	Count      int
}

func (c *ChangeEvent) Key() string {
	return c.ChangeName
}

func (c *ChangeEvent) Aggregate(other SamplableEvent) SamplableEvent {
	o := other.(*ChangeEvent)
	c.Count += o.Count
	return c
}

func (c *ChangeEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *ChangeEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "ChangeEvent")
	properties.Set("changeName", arena.NewString(c.ChangeName))
	properties.Set("sbType", arena.NewString(c.SbType))
	properties.Set("count", arena.NewNumberInt(c.Count))
	return event
}

type LinkPreviewStatusEvent struct {
	baseInfo
	StatusCode int
	ErrorMsg   string
}

func (l *LinkPreviewStatusEvent) Key() string {
	return fmt.Sprintf("linkpreview_status_%d", l.StatusCode)
}

func (l *LinkPreviewStatusEvent) Aggregate(other SamplableEvent) SamplableEvent {
	return other
}

func (l *LinkPreviewStatusEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (l *LinkPreviewStatusEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "LinkPreviewStatusEvent")
	properties.Set("statusCode", arena.NewNumberInt(l.StatusCode))
	properties.Set("errorMsg", arena.NewString(l.ErrorMsg))
	return event
}
