package importv2

import (
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Request carries the engine-level parameters of one run. Format-specific
// parameters (paths, tokens, per-format options) are given to the converter
// at construction time, not here.
type Request struct {
	SpaceID string
	Origin  objectorigin.ObjectOrigin
	Mode    Mode

	// UpdateExisting enables dedup matching by source file path — the
	// re-import identity switch (v1's updateExistingObjects).
	UpdateExisting bool

	// NoCollection suppresses the root collection and widget regardless of
	// the converter's RootSpec.
	NoCollection bool
}

// Result is the rich internal outcome of a run. The gRPC adapter maps it onto
// the wire notification/event; nothing here is wire-coupled.
type Result struct {
	RootCollectionId string
	WidgetLayout     model.BlockContentWidgetLayout

	Created int64
	Updated int64
	Skipped int64
	Failed  int64

	// Issues is capped at IssueCap entries; IssuesDropped counts overflow.
	Issues        []Issue
	IssuesDropped int64

	// Failure-path accounting: journal entries compensated vs. left behind
	// (with per-object detail in Issues).
	Compensated int
	Leaked      int

	// Err is the fatal error when the run aborted, nil otherwise.
	Err error
}

// IssueCap bounds the retained issue list in Result.
const IssueCap = 1000

// ObjectsCount is the number of objects materialized by the run (v1 semantics:
// excludes the root collection, which is reported separately).
func (r *Result) ObjectsCount() int64 {
	return r.Created + r.Updated
}
