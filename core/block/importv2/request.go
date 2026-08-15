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

	// ReportObjectId is the import report page (§16 item 1) — set when the
	// run produced issues and the page was persisted. Clients open it from
	// the finish notification/event and render a discard button on it.
	ReportObjectId string

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
	// CompensationRan means the engine actually executed compensation for
	// this failure (false on suspend, on success, on failures that never
	// reached — or were gated out of — the cleanup, and on aborts whose
	// journal was EMPTY: nothing ran there either). The disposal invariant
	// reads it: no path may destroy a run dir whose effects no compensation
	// covered, so Err != nil with CompensationRan == false keeps the dir for
	// the sweep instead of dropping the only record of what was created.
	CompensationRan bool
	// NothingToUndo means the abort found an EMPTY journal — no incarnation
	// put anything in the space, so compensation was vacuous and was skipped
	// together with its durable marker (review P0-B: the compensating
	// transition would scrub the manifest's crawl request and burn the dir's
	// crawl-resumable class to authorize zero deletes). It is a separate
	// proposition from CompensationRan on purpose: "there was nothing to
	// undo" does not mean "the dir is disposable" — a mid-crawl abort's dir
	// IS the crawl artifact DM-3 exists to keep. The adapter disposes such a
	// dir only when the user cancelled (the one intent that discards the
	// artifact) AND pass 3 never began; every other failure keeps it for the
	// sweep's attempts-capped retry. The second condition is not redundant:
	// this flag is an IN-MEMORY oracle, and past the manifest's sticky
	// MaterializeStarted marker the durable claim rows are compensation
	// inputs the journal never saw (review item 3).
	NothingToUndo bool

	// Suspended means the run was stopped by a graceful shutdown
	// (ErrSuspended cause) and deliberately NOT compensated — its durable
	// state is kept for the startup sweep. The engine is the single source
	// of this verdict; deriving it again from a context elsewhere can
	// disagree with what the engine actually did (a cancel cause is
	// one-shot: an inner abort followed by a Close reads differently from
	// the two contexts).
	Suspended bool

	// Cancelled means the USER stopped this run — the one intent that says
	// "discard this import" rather than "finish it later". It is a STOP
	// SOURCE, read from the cancel cause of the context the caller owns,
	// never from the fatal's code: a code is a shape, and both shapes lie
	// in opposite directions (review item 1). A transport deadline — the
	// Notion client's own http.Client{Timeout: time.Minute} — wears the
	// cancel's shape while nobody cancelled, so reading the code deleted a
	// two-hour crawl on a 60-second server hang; a cancelled Notion call
	// wears a retryable failure's shape, so reading the code kept a
	// cancelled import's dir, token intact, and silently re-ran it. Like
	// Suspended, the engine is the single source of this verdict.
	Cancelled bool

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
