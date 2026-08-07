// Package llmplan is the LLM-backed schemaplan.Planner (design doc
// docs/superpowers/specs/2026-08-07-importv2-whitelist-planner-design.md):
// one structured-output kinds call — the model only groups containers into
// kinds and names them — then schemaplan.CompleteKinds derives every property
// mapping in code. Invalid responses get exactly one corrective retry;
// context-starved runtimes degrade to per-container one-field calls; every
// other failure surfaces as the Plan error the converters degrade on.
package llmplan

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/core/ai/llmclient"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
)

// defaultBudget bounds the whole plan step, retries included — an import
// must never hang on a wedged endpoint. The kinds-only response is 75-85%
// smaller than the old full-plan one (~1,100-1,700 completion tokens on the
// measured 37-container workspace), but a local 5 tok/s decoder still sits
// near this budget's edge, and the per-container fallback shares it too.
const defaultBudget = 5 * time.Minute

// maxCompletionTokens caps the plan completion. A legitimate kinds response
// is ~1-2k tokens; an endpoint streaming more than this is broken or hostile.
// Kept deliberately roomy so a large workspace truncates the model, not us.
const maxCompletionTokens = 16384

type planner struct {
	client       *llmclient.Client
	budget       time.Duration
	effort       string
	perContainer bool
}

// Option configures the planner.
type Option func(*planner)

// WithBudget overrides the wall-clock budget for the whole plan step.
func WithBudget(budget time.Duration) Option {
	return func(p *planner) { p.budget = budget }
}

// WithReasoningEffort tunes a reasoning model's thinking, and switches it off
// entirely on a local thinking model ("none"). Models that do not know the
// parameter ignore it — the client drops it on rejection. Measured on the
// kinds task: "low" beat "high" (36/37 vs 31/37 containers typed) — the task
// is instruction-following-bound, not reasoning-bound.
func WithReasoningEffort(effort string) Option {
	return func(p *planner) { p.effort = effort }
}

// WithPerContainerCalls skips the global kinds call and goes straight to the
// tier-3 per-container planner (design §6), for providers known to be
// context-starved (Apple Foundation Models' ~4k total context cannot fit the
// global evidence at all). Without the option the per-container path still
// engages automatically when the kinds call reports truncation or a
// context-length overflow.
func WithPerContainerCalls() Option {
	return func(p *planner) { p.perContainer = true }
}

// New wraps an llmclient into a schemaplan.Planner.
func New(client *llmclient.Client, opts ...Option) schemaplan.Planner {
	p := &planner{client: client, budget: defaultBudget}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *planner) Plan(ctx context.Context, schemas []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
	ctx, cancel := context.WithTimeout(ctx, p.budget)
	defer cancel()

	if p.perContainer {
		kinds, err := p.planPerContainer(ctx, schemas)
		if err != nil {
			return schemaplan.Plan{}, fmt.Errorf("per-container plan: %w", err)
		}
		return schemaplan.CompleteKinds(kinds, schemas), nil
	}

	kinds, err := p.planKinds(ctx, schemas)
	if err != nil {
		if !isContextStarved(err) {
			return schemaplan.Plan{}, fmt.Errorf("kinds plan: %w", err)
		}
		// The degrade ladder: kinds call → per-container → naive (the caller's
		// llmPlanFailed path), each step keeping the shipped warning semantics.
		kinds, err = p.planPerContainer(ctx, schemas)
		if err != nil {
			return schemaplan.Plan{}, fmt.Errorf("per-container fallback: %w", err)
		}
	}
	return schemaplan.CompleteKinds(kinds, schemas), nil
}

// planKinds is the tier-1/2 path: one kinds call over the whole evidence,
// with one corrective retry on an invalid parse (budget shared).
func (p *planner) planKinds(ctx context.Context, schemas []schemaplan.ContainerSchema) ([]schemaplan.KindPlan, error) {
	userPrompt, aliases, err := renderKindsEvidence(schemas)
	if err != nil {
		return nil, fmt.Errorf("render kinds evidence: %w", err)
	}
	request := llmclient.Request{
		System:          kindsSystemPrompt(),
		User:            userPrompt,
		SchemaName:      "import_kinds",
		Schema:          kindsResponseSchema,
		MaxTokens:       maxCompletionTokens,
		ReasoningEffort: p.effort,
	}
	raw, _, err := p.client.CompleteJSON(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("kinds completion: %w", err)
	}
	kinds, parseErr := parseKinds(raw, aliases)
	if parseErr == nil {
		return kinds, nil
	}
	// One corrective retry with the error appended (path-addressed feedback,
	// the anyblockjson validate-after-generate pattern).
	request.User = userPrompt + "\n\nYour previous response was invalid: " + parseErr.Error() +
		"\nReturn corrected kinds following the schema exactly."
	raw, _, err = p.client.CompleteJSON(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("kinds completion retry: %w", err)
	}
	kinds, parseErr = parseKinds(raw, aliases)
	if parseErr != nil {
		return nil, fmt.Errorf("kinds response invalid twice: %w", parseErr)
	}
	return kinds, nil
}

// isContextStarved recognizes the failures that mean the global call can
// never fit this runtime: truncation at the token cap, or a 400 naming
// context length. Anything else (auth, unreachable, bad answers twice) would
// fail the per-container calls identically, so it propagates instead.
func isContextStarved(err error) bool {
	if errors.Is(err, llmclient.ErrResponseTruncated) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "context length") || strings.Contains(message, "context_length")
}
