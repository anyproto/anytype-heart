package schemaplan

import (
	"context"
	"fmt"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
)

// Reuse is the crawl-resume plan wiring both converters share (08-13 §6.3 /
// DM spec §8.3). Exactly one direction is active per run: a fresh durable
// run Records the sanitized plan it converts under; a resumed crawl gets
// that recording back as Preset and never calls the planner — LLM output is
// not deterministic across calls, and a second plan would mint divergent
// type/relation identities for the run's second half while the spool
// already holds the first half under the first plan.
type Reuse struct {
	// Preset is the recorded plan of a previous incarnation; non-nil
	// replaces the plan phase entirely (no planner call, no re-sanitize —
	// the recording is already sanitized output, and re-sanitizing would
	// duplicate its warnings against the rehydrated issue ledger).
	Preset *Plan
	// Record receives the sanitized plan of a fresh run BEFORE any object
	// is emitted under it. A record failure is fatal store trouble: a run
	// whose plan cannot be recorded must not spool objects a resume could
	// never reproduce (the a-run-that-cannot-journal rule).
	Record func(Plan) error
}

// Resolve is the plan phase's single source-of-truth rule, shared by the
// Notion and markdown converters so the reuse semantics cannot fork: reuse
// the preset when present; otherwise plan, degrade to the naive rules on
// planner failure (loudly, unless the run is being cancelled), sanitize,
// and record.
func Resolve(ctx context.Context, reuse Reuse, planner Planner, schemas []ContainerSchema, issue func(importv2.Issue)) (Plan, error) {
	if reuse.Preset != nil {
		return *reuse.Preset, nil
	}
	plan, err := planner.Plan(ctx, schemas)
	if err != nil {
		if ctx.Err() != nil {
			return Plan{}, fmt.Errorf("plan structure: %w", ctx.Err())
		}
		issue(importv2.Warning(importv2.IssueLLMPlanFailed, "plan",
			fmt.Sprintf("structure analysis unavailable (%s); imported with built-in rules", SummarizeError(err))))
		plan, _ = NewNaive().Plan(ctx, schemas)
	}
	plan = Sanitize(plan, schemas, issue)
	if reuse.Record != nil {
		if err := reuse.Record(plan); err != nil {
			return Plan{}, importv2.Fatal(importv2.IssueStoreError, fmt.Errorf("record structure plan: %w", err))
		}
	}
	return plan, nil
}
