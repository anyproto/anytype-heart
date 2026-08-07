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
	"sync"
	"time"

	"github.com/anyproto/anytype-heart/core/ai/llmclient"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/block/importv2/typesuggest"
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
	client           *llmclient.Client
	budget           time.Duration
	effort           string
	perContainer     bool
	chunkSize        int
	chunkConcurrency int
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

// WithChunkSize splits the evidence into chunks of at most n containers, one
// kinds call per chunk, merging the results by kind name. Zero (the default)
// sends the whole workspace in one call.
//
// The motivation is measured: coverage tracks corpus SIZE, not model or prompt.
// gemma4:e2b assigns every container on fixtures of 10-14 but only 32 of 37 on
// the real workspace, and neither an explicit count nor an instruction change
// recovered the gap. Chunking bounds what any single call has to enumerate.
//
// It is the same shape as WithPerContainerCalls (which is chunk size 1 with a
// one-field schema), but each chunk answers the full kinds schema, so grouping
// still happens inside a chunk. Cross-chunk grouping is recovered by merging
// kinds whose names normalize equal — and because the merged kind flows into
// CompleteKinds like any other, the coverage gate vetoes a cross-chunk merge
// that would bloat a type, exactly as it does within a chunk.
func WithChunkSize(n int) Option {
	return func(p *planner) { p.chunkSize = n }
}

// WithChunkConcurrency runs up to n chunk calls at once. Chunks are
// independent — each is a whole kinds call over its own slice — so this is
// pure wall-clock: on a cloud endpoint the chunked plan costs barely more than
// the single call it replaces. It defaults to 1 because a local single-GPU
// server serializes the work anyway, so parallelism there buys nothing and
// only multiplies peak memory.
//
// Merging stays deterministic regardless of completion order: results are
// collected per chunk and merged in chunk order, never arrival order.
func WithChunkConcurrency(n int) Option {
	return func(p *planner) { p.chunkConcurrency = n }
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

	if p.chunkSize > 0 && len(schemas) > p.chunkSize {
		kinds, err := p.planChunked(ctx, schemas)
		if err != nil {
			return schemaplan.Plan{}, fmt.Errorf("chunked plan: %w", err)
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

// planChunked runs one kinds call per chunk and merges the answers. A chunk
// that fails is skipped — its containers degrade to their typesuggest verdict
// in CompleteKinds — but every chunk failing is a failed plan, not an empty
// one, so the caller still gets its llmPlanFailed warning.
func (p *planner) planChunked(ctx context.Context, schemas []schemaplan.ContainerSchema) ([]schemaplan.KindPlan, error) {
	var (
		merged   []schemaplan.KindPlan
		byName   = map[string]int{}
		failures int
		lastErr  error
		chunks   int
	)
	bounds := balancedChunks(len(schemas), p.chunkSize)
	chunks = len(bounds)
	perChunk := make([][]schemaplan.KindPlan, len(bounds))
	chunkErrs := make([]error, len(bounds))

	concurrency := p.chunkConcurrency
	if concurrency < 1 {
		concurrency = 1
	}
	slots := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, bound := range bounds {
		// Acquire BEFORE launching so chunks start in order: at concurrency 1
		// this is exactly the sequential loop it replaces, and at higher
		// concurrency the endpoint still sees chunks begin in a stable order.
		slots <- struct{}{}
		wg.Add(1)
		go func(i int, bound [2]int) {
			defer wg.Done()
			defer func() { <-slots }()
			perChunk[i], chunkErrs[i] = p.planKinds(ctx, schemas[bound[0]:bound[1]])
		}(i, bound)
	}
	wg.Wait()

	// Merge in CHUNK order, not completion order, so the plan is identical
	// whatever the concurrency — planners must be deterministic for identical
	// input (the typesuggest seam rule).
	for i := range bounds {
		if err := chunkErrs[i]; err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("chunk %d: %w", i+1, err)
			}
			failures++
			lastErr = err
			continue
		}
		for _, kind := range perChunk[i] {
			// Kinds from different chunks merge when their names normalize
			// equal; the first chunk's spelling, icon and layout win, and
			// featured names accumulate (CompleteKinds caps and resolves them).
			key := typesuggest.Normalize(kind.Name)
			if key == "" {
				merged = append(merged, kind)
				continue
			}
			index, seen := byName[key]
			if !seen {
				byName[key] = len(merged)
				merged = append(merged, kind)
				continue
			}
			merged[index].ContainerIds = append(merged[index].ContainerIds, kind.ContainerIds...)
			merged[index].FeaturedNames = append(merged[index].FeaturedNames, kind.FeaturedNames...)
		}
	}
	if failures == chunks {
		return nil, fmt.Errorf("all %d chunks failed, last: %w", chunks, lastErr)
	}
	return merged, nil
}

// balancedChunks splits n items into ceil(n/max) chunks of near-equal size,
// returning [start,end) bounds. Plain fixed-size slicing leaves a remainder
// tail — 35 containers at 8 gives 8/8/8/8/3 — and a starved tail chunk names
// its containers with almost no comparative context, which is the condition
// that drives the model to copy source labels instead of naming the kind.
// Measured: at chunk size 8 (with a 3-container tail) singular==plural
// collisions rose from 1 to 6 against chunk size 12, whose 12/12/11 split has
// no starved tail. Balancing makes every chunk size behave like the good case.
func balancedChunks(n, max int) [][2]int {
	if n <= max {
		return [][2]int{{0, n}}
	}
	count := (n + max - 1) / max
	size, extra := n/count, n%count
	out := make([][2]int, 0, count)
	start := 0
	for i := 0; i < count; i++ {
		end := start + size
		if i < extra { // spread the remainder one item per chunk
			end++
		}
		out = append(out, [2]int{start, end})
		start = end
	}
	return out
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
