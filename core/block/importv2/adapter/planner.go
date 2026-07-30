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
	params.planner = llmplan.New(client)
	return params
}

func failingPlanner(err error) schemaplan.Planner {
	return schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
		return schemaplan.Plan{}, err
	})
}
