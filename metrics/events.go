package metrics

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
	ObjectId                string
	IndexLinksTimeMs        int64
	IndexDetailsTimeMs      int64
	IndexSetRelationsTimeMs int64
	RelationsCount          int
	DetailsCount            int
	SetRelationsCount       int
}

func (c IndexEvent) getBackend() MetricsBackend {
	return ampl
}

func (c IndexEvent) get() *anyEvent {
	if c.IndexLinksTimeMs+c.IndexDetailsTimeMs+c.IndexSetRelationsTimeMs < IndexEventThresholdMs {
		return nil
	}

	return &anyEvent{
		eventType: "index",
		eventData: map[string]interface{}{
			"object_id":     c.ObjectId,
			"links_ms":      c.IndexLinksTimeMs,
			"details_ms":    c.IndexDetailsTimeMs,
			"set_ms":        c.IndexSetRelationsTimeMs,
			"rel_count":     c.RelationsCount,
			"det_count":     c.DetailsCount,
			"set_rel_count": c.SetRelationsCount,
			"total_ms":      c.IndexLinksTimeMs + c.IndexDetailsTimeMs + c.IndexSetRelationsTimeMs,
		},
	}
}

const ReindexEventThresholdsMs = 100

type ReindexEvent struct {
	ReindexType    ReindexType
	Total          int
	Succeed        int
	SpentMs        int
	IndexesRemoved bool
}

func (c ReindexEvent) getBackend() MetricsBackend {
	return ampl
}

func (c ReindexEvent) get() *anyEvent {
	if c.SpentMs < ReindexEventThresholdsMs {
		return nil
	}
	return &anyEvent{
		eventType: "store_reindex",
		eventData: map[string]interface{}{
			"spent_ms":   c.SpentMs,
			"total":      c.Total,
			"failed":     c.Total - c.Succeed,
			"type":       c.ReindexType,
			"ix_removed": c.IndexesRemoved,
		},
	}
}

const BlockSplitEventThresholdsMs = 10

type BlockSplit struct {
	AlgorithmMs int64
	ApplyMs     int64
	ObjectId    string
}

func (c BlockSplit) getBackend() MetricsBackend {
	return ampl
}

func (c BlockSplit) get() *anyEvent {
	if c.ApplyMs+c.AlgorithmMs < BlockSplitEventThresholdsMs {
		return nil
	}

	return &anyEvent{
		eventType: "block_merge",
		eventData: map[string]interface{}{
			"object_id":    c.ObjectId,
			"algorithm_ms": c.AlgorithmMs,
			"apply_ms":     c.ApplyMs,
			"total_ms":     c.AlgorithmMs + c.ApplyMs,
		},
	}
}

type TreeBuild struct {
	SbType   uint64
	TimeMs   int64
	ObjectId string
	Logs     int
	Request  string
}

func (c TreeBuild) getBackend() MetricsBackend {
	return ampl
}

func (c TreeBuild) get() *anyEvent {
	return &anyEvent{
		eventType: "tree_build",
		eventData: map[string]interface{}{
			"object_id": c.ObjectId,
			"logs":      c.Logs,
			"request":   c.Request,
			"time_ms":   c.TimeMs,
			"sb_type":   c.SbType,
		},
	}
}

const StateApplyThresholdMs = 100

type StateApply struct {
	BeforeApplyMs  int64
	StateApplyMs   int64
	PushChangeMs   int64
	ReportChangeMs int64
	ApplyHookMs    int64
	ObjectId       string
}

func (c StateApply) getBackend() MetricsBackend {
	return ampl
}

func (c StateApply) get() *anyEvent {
	total := c.StateApplyMs + c.PushChangeMs + c.BeforeApplyMs + c.ApplyHookMs + c.ReportChangeMs
	if total <= StateApplyThresholdMs {
		return nil
	}
	return &anyEvent{
		eventType: "state_apply",
		eventData: map[string]interface{}{
			"before_ms": c.BeforeApplyMs,
			"apply_ms":  c.StateApplyMs,
			"push_ms":   c.PushChangeMs,
			"report_ms": c.ReportChangeMs,
			"hook_ms":   c.ApplyHookMs,
			"object_id": c.ObjectId,
			"total_ms":  c.StateApplyMs + c.PushChangeMs + c.BeforeApplyMs + c.ApplyHookMs + c.ReportChangeMs,
		},
	}
}

type AppStart struct {
	Request   string
	TotalMs   int64
	PerCompMs map[string]int64
	Extra     map[string]interface{}
}

func (c AppStart) getBackend() MetricsBackend {
	return ampl
}

func (c AppStart) get() *anyEvent {
	ev := &anyEvent{
		eventType: "app_start",
		eventData: map[string]interface{}{
			"request": c.Request,
			"time_ms": c.TotalMs,
		},
	}

	for comp, ms := range c.PerCompMs {
		ev.eventData["spent_"+comp] = ms
	}
	for key, val := range c.Extra {
		ev.eventData[key] = val
	}
	return ev
}

type BlockMerge struct {
	AlgorithmMs int64
	ApplyMs     int64
	ObjectId    string
}

func (c BlockMerge) getBackend() MetricsBackend {
	return ampl
}

func (c BlockMerge) get() *anyEvent {
	return &anyEvent{
		eventType: "block_split",
		eventData: map[string]interface{}{
			"object_id":    c.ObjectId,
			"algorithm_ms": c.AlgorithmMs,
			"apply_ms":     c.ApplyMs,
			"total_ms":     c.AlgorithmMs + c.ApplyMs,
		},
	}
}

type CreateObjectEvent struct {
	SetDetailsMs            int64
	GetWorkspaceBlockWaitMs int64
	WorkspaceCreateMs       int64
	SmartblockCreateMs      int64
	SmartblockType          int
	ObjectId                string
}

func (c CreateObjectEvent) getBackend() MetricsBackend {
	return ampl
}

func (c CreateObjectEvent) get() *anyEvent {
	return &anyEvent{
		eventType: "create_object",
		eventData: map[string]interface{}{
			"set_details_ms":              c.SetDetailsMs,
			"get_workspace_block_wait_ms": c.GetWorkspaceBlockWaitMs,
			"workspace_create_ms":         c.WorkspaceCreateMs,
			"smartblock_create_ms":        c.SmartblockCreateMs,
			"total_ms":                    c.SetDetailsMs + c.GetWorkspaceBlockWaitMs + c.WorkspaceCreateMs + c.SmartblockCreateMs,
			"smartblock_type":             c.SmartblockType,
			"object_id":                   c.ObjectId,
		},
	}
}

type OpenBlockEvent struct {
	GetBlockMs     int64
	DataviewMs     int64
	ApplyMs        int64
	ShowMs         int64
	FileWatcherMs  int64
	SmartblockType int
	ObjectId       string
}

func (c OpenBlockEvent) getBackend() MetricsBackend {
	return ampl
}

func (c OpenBlockEvent) get() *anyEvent {
	return &anyEvent{
		eventType: "open_block",
		eventData: map[string]interface{}{
			"object_id":          c.ObjectId,
			"get_block_ms":       c.GetBlockMs,
			"dataview_notify_ms": c.DataviewMs,
			"apply_ms":           c.ApplyMs,
			"show_ms":            c.ShowMs,
			"file_watchers_ms":   c.FileWatcherMs,
			"total_ms":           c.GetBlockMs + c.DataviewMs + c.ApplyMs + c.ShowMs + c.FileWatcherMs,
			"smartblock_type":    c.SmartblockType,
		},
	}
}


type ImportStartedEvent struct {
	ID         string
	ImportType string
}

func (c ImportStartedEvent) getBackend() MetricsBackend {
	return ampl
}

func (i ImportStartedEvent) ToEvent() *Event {
	return &Event{
		eventType: "import_started",
		eventData: map[string]interface{}{
			"import_id":   i.ID,
			"import_type": i.ImportType,
		},
	}
}

type ImportFinishedEvent struct {
	ID         string
	ImportType string
}

func (i ImportFinishedEvent) ToEvent() *Event {
	return &Event{
		eventType: "import_finished",
		eventData: map[string]interface{}{
			"import_id":   i.ID,
			"import_type": i.ImportType,
		},
	}
}
