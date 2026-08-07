package adapter

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/ai/llmclient"
	"github.com/anyproto/anytype-heart/core/block/importv2/llmplan"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/pb"
)

// plannerParams is the adapter-resolved BYOK enrichment config
// (docs/ImportV2LLM.md §9). The zero value means feature off — converters
// default to the naive planner.
type plannerParams struct {
	planner        schemaplan.Planner
	includeSamples bool
}

// plannerFromRequest builds the run's planner from the request's aiParams.
// A present-but-unusable config (openai without a token, unknown provider)
// still returns a planner: one that fails on first use, so the run degrades
// through the converter's llmPlanFailed path instead of silently ignoring the
// user's explicit ask.
func plannerFromRequest(req *pb.RpcObjectImportRequest) plannerParams {
	aiParams := req.GetAiParams()
	if aiParams == nil {
		return plannerParams{}
	}
	params := plannerParams{includeSamples: aiParams.GetIncludeContentSamples()}
	cfg, on, err := llmclient.FromProto(aiParams.GetConfig())
	if err != nil {
		params.planner = failingPlanner(fmt.Errorf("ai config: %w", err))
		return params
	}
	if !on {
		return plannerParams{}
	}
	client, err := llmclient.New(cfg)
	if err != nil {
		params.planner = failingPlanner(fmt.Errorf("ai client: %w", err))
		return params
	}
	// "low" is meaningful on OpenAI and inert on ollama, where "low" and
	// "high" are byte-identical requests and only "none" differs. It is kept
	// as a cost-conscious default, NOT on the strength of the retracted
	// "low beat high" measurement — see the spec's §1 retraction. Thinking
	// itself is load-bearing: switching it off makes the model stop
	// abstracting and copy source labels into both name fields.
	params.planner = llmplan.New(client,
		llmplan.WithReasoningEffort("low"),
		llmplan.WithChunkSize(planChunkSize),
		llmplan.WithChunkConcurrency(chunkConcurrencyFor(aiParams.GetConfig())),
	)
	return params
}

// planChunkSize bounds how many containers one kinds call has to enumerate.
//
// Measured on the real workspace (gemma4:e2b): coverage tracks corpus SIZE
// rather than model or prompt — every 10-14-container fixture was assigned in
// full, the 35-container workspace only 32. Chunking at 8 closed that
// completely (35/35) and, unexpectedly, roughly halved the rate at which the
// model names a type after its source database instead of naming the kind
// (81% → 36%); a smaller call leaves it enough budget to name rather than
// copy. On gpt-5.6-luna, which already assigned everything, chunking still cut
// that rate (~42% → ~31%).
//
// Chunks are balanced, so 35 containers run as 7×5 rather than 8/8/8/8/3 — a
// starved tail chunk has almost no comparative context and regresses naming
// (singular==plural collisions went 1 → 6 on the unbalanced split).
const planChunkSize = 8

// Chunk calls are independent, so concurrency is pure wall-clock. On a cloud
// endpoint the chunked plan is *cheaper* than the single call it replaces
// (measured on luna: 25s sequential → 9s at 5, against 22s unchunked). A local
// server serializes the work regardless, so parallelism there buys nothing and
// only multiplies peak memory.
const (
	cloudChunkConcurrency = 5
	localChunkConcurrency = 1
)

// chunkConcurrencyFor picks the concurrency from the provider the user chose.
// The distinction that matters is whether the endpoint can serve requests in
// parallel, not where it is: a self-hosted server on another machine is still
// one GPU taking one request at a time. The provider enum is the user's own
// declaration of which kind of endpoint this is, so it is the right signal —
// an OpenAI-compatible local server is configured as its own provider even
// when the endpoint is overridden.
func chunkConcurrencyFor(cfg *pb.RpcAIProviderConfig) int {
	if cfg.GetProvider() == pb.RpcAI_OPENAI {
		return cloudChunkConcurrency
	}
	return localChunkConcurrency
}

func failingPlanner(err error) schemaplan.Planner {
	return schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
		return schemaplan.Plan{}, err
	})
}
