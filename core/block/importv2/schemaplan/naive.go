package schemaplan

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/importv2/typesuggest"
)

// NewNaive wraps the typesuggest keyword/shape rules as a Planner: one type
// verdict per container, no property normalization. This is the fallback for
// runs without LLM config and for every LLM failure (docs/ImportV2LLM.md §7).
func NewNaive() Planner {
	return naivePlanner{suggestor: typesuggest.NewNaive()}
}

type naivePlanner struct {
	suggestor typesuggest.Suggestor
}

func (p naivePlanner) Plan(_ context.Context, schemas []ContainerSchema) (Plan, error) {
	plan := Plan{}
	for _, schema := range schemas {
		evidence := typesuggest.Evidence{ContainerName: schema.Name}
		for _, property := range schema.Properties {
			evidence.Properties = append(evidence.Properties, typesuggest.Property{
				Name:   property.Name,
				Format: property.Format,
			})
		}
		suggestion, ok := p.suggestor.Suggest(evidence)
		if !ok {
			continue
		}
		if plan.Containers == nil {
			plan.Containers = make(map[string]ContainerPlan)
		}
		plan.Containers[schema.Id] = ContainerPlan{
			TypeKey: suggestion.TypeKey,
			Reason:  suggestion.Reason,
		}
	}
	return plan, nil
}
