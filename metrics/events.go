package metrics

import (
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

const IndexEventThresholdMs = 10

type IndexEvent struct {
	baseInfo
	ObjectId                string
	IndexLinksTimeMs        int64
	IndexDetailsTimeMs      int64
	IndexSetRelationsTimeMs int64
	RelationsCount          int
	DetailsCount            int
	SetRelationsCount       int
}

func (c *IndexEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *IndexEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	if c.IndexLinksTimeMs+c.IndexDetailsTimeMs+c.IndexSetRelationsTimeMs < IndexEventThresholdMs {
		return nil
	}

	event, properties := setupProperties(arena, "index")

	properties.Set("object_id", arena.NewString(c.ObjectId))
	properties.Set("links_ms", arena.NewNumberInt(int(c.IndexLinksTimeMs)))
	properties.Set("details_ms", arena.NewNumberInt(int(c.IndexDetailsTimeMs)))
	properties.Set("set_ms", arena.NewNumberInt(int(c.IndexSetRelationsTimeMs)))
	properties.Set("rel_count", arena.NewNumberInt(c.RelationsCount))
	properties.Set("det_count", arena.NewNumberInt(c.DetailsCount))
	properties.Set("set_rel_count", arena.NewNumberInt(c.SetRelationsCount))
	properties.Set("total_ms", arena.NewNumberInt(int(c.IndexLinksTimeMs+c.IndexDetailsTimeMs+c.IndexSetRelationsTimeMs)))

	return event
}

const ReindexEventThresholdsMs = 100

type ReindexEvent struct {
	baseInfo
	ReindexType    ReindexType
	Total          int
	Succeed        int
	SpentMs        int
	IndexesRemoved bool
}

func (c *ReindexEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *ReindexEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	if c.SpentMs < ReindexEventThresholdsMs {
		return nil
	}

	event, properties := setupProperties(arena, "store_reindex")

	properties.Set("spent_ms", arena.NewNumberInt(c.SpentMs))
	properties.Set("total", arena.NewNumberInt(c.Total))
	properties.Set("failed", arena.NewNumberInt(c.Total-c.Succeed))
	properties.Set("type", arena.NewNumberInt(int(c.ReindexType)))
	var isRemoved *fastjson.Value
	if c.IndexesRemoved {
		isRemoved = arena.NewTrue()
	} else {
		isRemoved = arena.NewFalse()
	}
	properties.Set("ix_removed", isRemoved)

	return event
}

const BlockSplitEventThresholdsMs = 10

type BlockSplit struct {
	baseInfo
	AlgorithmMs int64
	ApplyMs     int64
	ObjectId    string
}

func (c *BlockSplit) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *BlockSplit) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	if c.ApplyMs+c.AlgorithmMs < BlockSplitEventThresholdsMs {
		return nil
	}

	event, properties := setupProperties(arena, "block_merge")

	properties.Set("object_id", arena.NewString(c.ObjectId))
	properties.Set("algorithm_ms", arena.NewNumberInt(int(c.AlgorithmMs)))
	properties.Set("apply_ms", arena.NewNumberInt(int(c.ApplyMs)))
	properties.Set("total_ms", arena.NewNumberInt(int(c.AlgorithmMs+c.ApplyMs)))

	return event
}

type TreeBuild struct {
	baseInfo
	SbType   uint64
	TimeMs   int64
	ObjectId string
	Logs     int
	Request  string
}

func (c *TreeBuild) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *TreeBuild) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "tree_build")

	properties.Set("object_id", arena.NewString(c.ObjectId))
	properties.Set("logs", arena.NewNumberInt(c.Logs))
	properties.Set("request", arena.NewString(c.Request))
	properties.Set("time_ms", arena.NewNumberInt(int(c.TimeMs)))
	properties.Set("sb_type", arena.NewNumberInt(int(c.SbType)))

	return event
}

func setupProperties(arena *fastjson.Arena, eventType string) (*fastjson.Value, *fastjson.Value) {
	event := arena.NewObject()
	properties := arena.NewObject()
	event.Set("event_type", arena.NewString(eventType))
	event.Set("event_properties", properties)
	return event, properties
}

const StateApplyThresholdMs = 100

type StateApply struct {
	baseInfo
	BeforeApplyMs  int64
	StateApplyMs   int64
	PushChangeMs   int64
	ReportChangeMs int64
	ApplyHookMs    int64
	ObjectId       string
}

func (c *StateApply) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *StateApply) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	total := c.StateApplyMs + c.PushChangeMs + c.BeforeApplyMs + c.ApplyHookMs + c.ReportChangeMs
	if total <= StateApplyThresholdMs {
		return nil
	}
	event, properties := setupProperties(arena, "state_apply")

	properties.Set("before_ms", arena.NewNumberInt(int(c.BeforeApplyMs)))
	properties.Set("apply_ms", arena.NewNumberInt(int(c.StateApplyMs)))
	properties.Set("push_ms", arena.NewNumberInt(int(c.PushChangeMs)))
	properties.Set("report_ms", arena.NewNumberInt(int(c.ReportChangeMs)))
	properties.Set("hook_ms", arena.NewNumberInt(int(c.ApplyHookMs)))
	properties.Set("object_id", arena.NewString(c.ObjectId))
	properties.Set("total_ms", arena.NewNumberInt(int(c.StateApplyMs+c.PushChangeMs+c.BeforeApplyMs+c.ApplyHookMs+c.ReportChangeMs)))

	return event
}

type AppStart struct {
	baseInfo
	Request   string
	TotalMs   int64
	PerCompMs map[string]int64
	Extra     map[string]interface{}
}

func (c *AppStart) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *AppStart) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "app_start")

	properties.Set("request", arena.NewString(c.Request))
	properties.Set("time_ms", arena.NewNumberInt(int(c.TotalMs)))
	for comp, ms := range c.PerCompMs {
		properties.Set("spent_"+comp, arena.NewNumberInt(int(ms)))
	}

	for key, val := range c.Extra {
		switch val := val.(type) {
		case string:
			properties.Set(key, arena.NewString(val))
		case int64:
			properties.Set(key, arena.NewNumberInt(int(val)))
		}
	}

	return event
}

type BlockMerge struct {
	baseInfo
	AlgorithmMs int64
	ApplyMs     int64
	ObjectId    string
}

func (c *BlockMerge) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *BlockMerge) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "block_split")

	properties.Set("object_id", arena.NewString(c.ObjectId))
	properties.Set("algorithm_ms", arena.NewNumberInt(int(c.AlgorithmMs)))
	properties.Set("apply_ms", arena.NewNumberInt(int(c.ApplyMs)))
	properties.Set("total_ms", arena.NewNumberInt(int(c.AlgorithmMs+c.ApplyMs)))

	return event
}

type CreateObjectEvent struct {
	baseInfo
	SetDetailsMs            int64
	GetWorkspaceBlockWaitMs int64
	WorkspaceCreateMs       int64
	SmartblockCreateMs      int64
	SmartblockType          int
	ObjectId                string
}

func (c *CreateObjectEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *CreateObjectEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "create_object")

	properties.Set("set_details_ms", arena.NewNumberInt(int(c.SetDetailsMs)))
	properties.Set("get_workspace_block_wait_ms", arena.NewNumberInt(int(c.GetWorkspaceBlockWaitMs)))
	properties.Set("workspace_create_ms", arena.NewNumberInt(int(c.WorkspaceCreateMs)))
	properties.Set("smartblock_create_ms", arena.NewNumberInt(int(c.SmartblockCreateMs)))
	properties.Set("total_ms", arena.NewNumberInt(int(c.SetDetailsMs+c.GetWorkspaceBlockWaitMs+c.WorkspaceCreateMs+c.SmartblockCreateMs)))
	properties.Set("smartblock_type", arena.NewNumberInt(c.SmartblockType))
	properties.Set("object_id", arena.NewString(c.ObjectId))

	return event
}

type OpenBlockEvent struct {
	baseInfo
	GetBlockMs     int64
	DataviewMs     int64
	ApplyMs        int64
	ShowMs         int64
	FileWatcherMs  int64
	SmartblockType int
	ObjectId       string
}

func (c *OpenBlockEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *OpenBlockEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "open_block")

	properties.Set("object_id", arena.NewString(c.ObjectId))
	properties.Set("get_block_ms", arena.NewNumberInt(int(c.GetBlockMs)))
	properties.Set("dataview_notify_ms", arena.NewNumberInt(int(c.DataviewMs)))
	properties.Set("apply_ms", arena.NewNumberInt(int(c.ApplyMs)))
	properties.Set("show_ms", arena.NewNumberInt(int(c.ShowMs)))
	properties.Set("file_watchers_ms", arena.NewNumberInt(int(c.FileWatcherMs)))
	properties.Set("total_ms", arena.NewNumberInt(int(c.GetBlockMs+c.DataviewMs+c.ApplyMs+c.ShowMs+c.FileWatcherMs)))
	properties.Set("smartblock_type", arena.NewNumberInt(c.SmartblockType))

	return event
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

type LongMethodEvent struct {
	baseInfo
	methodName string
	middleTime int64
	stack      string
}

func (c *LongMethodEvent) GetBackend() anymetry.MetricsBackend {
	return inhouse
}

func (c *LongMethodEvent) MarshalFastJson(arena *fastjson.Arena) anymetry.JsonEvent {
	event, properties := setupProperties(arena, "LongMethodEvent")

	properties.Set("methodName", arena.NewString(c.methodName))
	properties.Set("middleTime", arena.NewNumberInt(int(c.middleTime)))
	properties.Set("stack", arena.NewString(c.stack))
	return event
}
