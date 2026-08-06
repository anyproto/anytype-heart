// Package llmplan is the LLM-backed schemaplan.Planner
// (docs/ImportV2LLM.md §5-6): one structured-output completion over the
// run's container schemas, rendered in the anyblockjson objectType
// vocabulary, parsed back into a plan. Invalid responses get exactly one
// corrective retry; every other failure surfaces as the Plan error the
// converters degrade on.
package llmplan

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/ai/llmclient"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// defaultBudget bounds the whole plan step, retries included — an import
// must never hang on a wedged endpoint.
//
// Measured on a real 37-container workspace (2026-08-06, gpt-4o-mini): one
// plan call took 72s on one run and exceeded 90s on the next, same prompt and
// evidence. Always-mint makes the model write a type definition per kind, so
// the completion is far larger than when types were the exception. At 90s the
// step failed outright about half the time, and the corrective retry — which
// shares this deadline — had no room at all. Sized for two calls.
const defaultBudget = 5 * time.Minute

// maxCompletionTokens caps the plan completion. A legitimate plan is a few
// KB; an endpoint streaming more than this is broken or hostile. The same
// measured run used 6720 completion tokens for 37 containers — 82% of the
// former 8192 cap — so a larger workspace would have been truncated.
const maxCompletionTokens = 16384

type planner struct {
	client *llmclient.Client
	budget time.Duration
	effort string
}

// Option configures the planner.
type Option func(*planner)

// WithBudget overrides the wall-clock budget for the whole plan step.
func WithBudget(budget time.Duration) Option {
	return func(p *planner) { p.budget = budget }
}

// WithReasoningEffort tunes a reasoning model's thinking, and switches it off
// entirely on a local thinking model ("none"). Models that do not know the
// parameter ignore it — the client drops it on rejection.
func WithReasoningEffort(effort string) Option {
	return func(p *planner) { p.effort = effort }
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

	userPrompt, err := renderSchemas(schemas)
	if err != nil {
		return schemaplan.Plan{}, fmt.Errorf("render schemas: %w", err)
	}
	request := llmclient.Request{
		System:          systemPrompt(),
		User:            userPrompt,
		SchemaName:      "import_plan",
		Schema:          responseSchema,
		MaxTokens:       maxCompletionTokens,
		ReasoningEffort: p.effort,
	}
	raw, _, err := p.client.CompleteJSON(ctx, request)
	if err != nil {
		return schemaplan.Plan{}, fmt.Errorf("plan completion: %w", err)
	}
	plan, parseErr := parsePlan(raw)
	if parseErr == nil {
		return plan, nil
	}
	// One corrective retry with the error appended (path-addressed feedback,
	// the anyblockjson validate-after-generate pattern).
	request.User = userPrompt + "\n\nYour previous response was invalid: " + parseErr.Error() +
		"\nReturn a corrected plan following the schema exactly."
	raw, _, err = p.client.CompleteJSON(ctx, request)
	if err != nil {
		return schemaplan.Plan{}, fmt.Errorf("plan completion retry: %w", err)
	}
	plan, parseErr = parsePlan(raw)
	if parseErr != nil {
		return schemaplan.Plan{}, fmt.Errorf("plan response invalid twice: %w", parseErr)
	}
	return plan, nil
}

// wire types — the response contract. Strict structured output requires every
// field present; absence is spelled "".
type wirePlan struct {
	Types      []wireType      `json:"types"`
	Containers []wireContainer `json:"containers"`
}

type wireType struct {
	Key            string             `json:"key"`
	Name           string             `json:"name"`
	PluralName     string             `json:"pluralName"`
	Icon           string             `json:"icon"`
	Layout         string             `json:"layout"`
	TypeProperties []wireTypeProperty `json:"typeProperties"`
}

type wireTypeProperty struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Format  string `json:"format"`
	Section string `json:"section"`
}

type wireContainer struct {
	Id         string            `json:"id"`
	Type       string            `json:"type"`
	Properties []wirePropertyMap `json:"properties"`
}

type wirePropertyMap struct {
	Id     string `json:"id"`
	Key    string `json:"key"`
	Name   string `json:"name"`
	Format string `json:"format"`
}

func parsePlan(raw json.RawMessage) (schemaplan.Plan, error) {
	var wire wirePlan
	decoder := json.NewDecoder(bytesReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return schemaplan.Plan{}, fmt.Errorf("decode plan: %w", err)
	}

	plan := schemaplan.Plan{}
	for _, wireDef := range wire.Types {
		if wireDef.Key == "" || wireDef.Name == "" {
			continue
		}
		def := schemaplan.TypeDefinition{
			Key:        domain.TypeKey(wireDef.Key),
			Name:       wireDef.Name,
			PluralName: wireDef.PluralName,
			IconName:   wireDef.Icon,
			Layout:     layoutOf(wireDef.Layout),
		}
		for _, prop := range wireDef.TypeProperties {
			if prop.Key == "" {
				continue
			}
			def.Properties = append(def.Properties, schemaplan.TypeProperty{
				Key:      domain.RelationKey(prop.Key),
				Name:     prop.Name,
				Format:   formatOf(prop.Format),
				Featured: prop.Section == "featured",
			})
		}
		plan.NewTypes = append(plan.NewTypes, def)
	}
	for _, container := range wire.Containers {
		if container.Id == "" {
			continue
		}
		containerPlan := schemaplan.ContainerPlan{
			TypeKey: domain.TypeKey(container.Type),
			Reason:  "LLM plan",
		}
		for _, prop := range container.Properties {
			if prop.Id == "" || prop.Key == "" {
				continue
			}
			if containerPlan.Properties == nil {
				containerPlan.Properties = map[string]schemaplan.PropertyPlan{}
			}
			containerPlan.Properties[prop.Id] = schemaplan.PropertyPlan{
				Key:    domain.RelationKey(prop.Key),
				Name:   prop.Name,
				Format: formatOf(prop.Format),
			}
		}
		if containerPlan.TypeKey == "" && len(containerPlan.Properties) == 0 {
			continue
		}
		if plan.Containers == nil {
			plan.Containers = map[string]schemaplan.ContainerPlan{}
		}
		plan.Containers[container.Id] = containerPlan
	}
	return plan, nil
}

func layoutOf(name string) model.ObjectTypeLayout {
	switch name {
	case "todo":
		return model.ObjectType_todo
	case "profile":
		return model.ObjectType_profile
	case "note":
		return model.ObjectType_note
	default:
		return model.ObjectType_basic
	}
}
