package notion

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func runScriptedWithOptions(t *testing.T, opts ...Option) *recordingSink {
	t.Helper()
	server := httptest.NewServer(scriptedWorkspace(t))
	t.Cleanup(server.Close)
	apiClient := client.NewClient("token",
		client.WithBaseURL(server.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir(), opts...)
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)
	return sink
}

func issueMessages(sink *recordingSink, code importv2.IssueCode) []string {
	var out []string
	for _, issue := range sink.issues {
		if issue.Code == code {
			out = append(out, issue.Message)
		}
	}
	return out
}

func TestScriptedPlan(t *testing.T) {
	t.Run("plan types the container onto a new type and remaps a property", func(t *testing.T) {
		// given — the scripted workspace's db1 (Tasks: Priority/Tags/Score)
		plan := schemaplan.Plan{
			NewTypes: []schemaplan.TypeDefinition{{
				Key: "sprint", Name: "Sprint", Layout: model.ObjectType_todo,
				Properties: []schemaplan.TypeProperty{
					{Key: bundle.RelationKeyDueDate, Featured: true},
					{Key: "effort", Name: "Effort", Format: model.RelationFormat_number},
				},
			}},
			Containers: map[string]schemaplan.ContainerPlan{
				"db1": {
					TypeKey: "sprint",
					Reason:  "LLM plan",
					Properties: map[string]schemaplan.PropertyPlan{
						"score": {Key: "effort", Name: "Effort", Format: model.RelationFormat_number},
					},
				},
			},
		}
		planner := schemaplan.PlannerFunc(func(_ context.Context, schemas []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			require.Len(t, schemas, 1)
			assert.Equal(t, "db1", schemas[0].Id)
			assert.Equal(t, "Tasks", schemas[0].Name)
			return plan, nil
		})

		// when
		sink := runScriptedWithOptions(t, WithPlanner(planner))

		// then — the plan type is emitted with its minted key
		mintedType := schemaplan.CustomTypeKey("sprint")
		typeObject := sink.byKey(schemaplan.TypeSourceKey("sprint"))
		require.NotNil(t, typeObject, "plan type object emitted")
		assert.Equal(t, "Sprint", typeObject.Payload.Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, int64(model.ObjectType_todo), typeObject.Payload.Details.GetInt64(bundle.RelationKeyRecommendedLayout))
		assert.Equal(t, []string{bundle.RelationKeyDueDate.BundledURL()},
			typeObject.Payload.Details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))

		// the type definition's custom property exists exactly once, under the
		// shared plan key, and the container's Score resolves onto it
		effortKey := schemaplan.CustomRelationKey("effort").String()
		effort := sink.byKey("relation:" + effortKey)
		require.NotNil(t, effort, "plan relation emitted before use")
		assert.Equal(t, "Effort", effort.Payload.Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, int64(model.RelationFormat_number), effort.Payload.Details.GetInt64(bundle.RelationKeyRelationFormat))
		assert.Nil(t, sink.relationByName("Score"), "Score does not mint its own relation")

		// database rows carry the minted type
		page := sink.byKey("p1")
		require.NotNil(t, page)
		assert.Equal(t, []string{mintedType.String()}, page.Payload.ObjectTypes)
		assert.Equal(t, 4.5, page.Payload.Details.GetFloat64(domain.RelationKey(effortKey)),
			"page value follows the remapped relation")

		// unplanned properties import as before
		assert.NotNil(t, sink.relationByName("Priority"))

		// observability: typeSuggested with the plan reason + propertyMapped
		suggested := issueMessages(sink, importv2.IssueTypeSuggested)
		require.Len(t, suggested, 1)
		assert.Contains(t, suggested[0], `database "Tasks" pages imported as `+mintedType.String()+` (LLM plan)`)
		mapped := issueMessages(sink, importv2.IssuePropertyMapped)
		require.Len(t, mapped, 1)
		assert.Contains(t, mapped[0], `property "Score" imported as "Effort"`)
	})

	t.Run("bundled remap reuses the bundled relation", func(t *testing.T) {
		// given — remap the Priority select onto the bundled tag relation
		planner := schemaplan.PlannerFunc(func(_ context.Context, schemas []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			return schemaplan.Plan{Containers: map[string]schemaplan.ContainerPlan{
				"db1": {Properties: map[string]schemaplan.PropertyPlan{
					"prio": {Key: bundle.RelationKeyTag},
				}},
			}}, nil
		})

		// when
		sink := runScriptedWithOptions(t, WithPlanner(planner))

		// then — no new relation for Priority; its options land on the tag key
		assert.Nil(t, sink.relationByName("Priority"))
		high := sink.byKey("option:tag:High")
		require.NotNil(t, high, "remapped select options attach to the bundled key")
	})

	t.Run("select remapped onto a number-format type property drops safely", func(t *testing.T) {
		// given — the reviewed corruption scenario: db1's Priority (select →
		// status format) onto a type property declared number
		planner := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			return schemaplan.Plan{
				NewTypes: []schemaplan.TypeDefinition{{
					Key: "sprint", Name: "Sprint",
					Properties: []schemaplan.TypeProperty{{Key: "effort", Name: "Effort", Format: model.RelationFormat_number}},
				}},
				Containers: map[string]schemaplan.ContainerPlan{
					"db1": {TypeKey: "sprint", Properties: map[string]schemaplan.PropertyPlan{
						"prio": {Key: "effort"},
					}},
				},
			}, nil
		})

		// when
		sink := runScriptedWithOptions(t, WithPlanner(planner))

		// then — the entry dropped at sanitize; Priority imports unmapped
		dropped := issueMessages(sink, importv2.IssueLLMPlanEntryDropped)
		require.NotEmpty(t, dropped)
		priority := sink.relationByName("Priority")
		require.NotNil(t, priority, "Priority keeps its own relation")
		assert.Equal(t, int64(model.RelationFormat_status), priority.Payload.Details.GetInt64(bundle.RelationKeyRelationFormat))
		// the Effort relation, if emitted for the type, stays number and holds
		// no option values
		effortKey := schemaplan.CustomRelationKey("effort").String()
		page := sink.byKey("p1")
		require.NotNil(t, page)
		assert.False(t, page.Payload.Details.Has(domain.RelationKey(effortKey)),
			"no select values may land under the number relation")
	})

	t.Run("planner failure degrades to naive with a warning", func(t *testing.T) {
		// given
		planner := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			return schemaplan.Plan{}, errors.New("endpoint unreachable")
		})

		// when
		sink := runScriptedWithOptions(t, WithPlanner(planner))

		// then — one llmPlanFailed warning, and the naive verdict still lands
		failed := issueMessages(sink, importv2.IssueLLMPlanFailed)
		require.Len(t, failed, 1)
		assert.Contains(t, failed[0], "imported with built-in rules")
		suggested := issueMessages(sink, importv2.IssueTypeSuggested)
		require.Len(t, suggested, 1)
		assert.True(t, strings.Contains(suggested[0], "imported as task (container name)"), suggested[0])
	})

	t.Run("hallucinated plan entries are dropped loudly, import unharmed", func(t *testing.T) {
		// given
		planner := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			return schemaplan.Plan{Containers: map[string]schemaplan.ContainerPlan{
				"db1":  {TypeKey: "unicornType"},
				"db99": {TypeKey: bundle.TypeKeyTask},
			}}, nil
		})

		// when
		sink := runScriptedWithOptions(t, WithPlanner(planner))

		// then
		dropped := issueMessages(sink, importv2.IssueLLMPlanEntryDropped)
		assert.Len(t, dropped, 2)
		page := sink.byKey("p1")
		require.NotNil(t, page)
		assert.Equal(t, []string{bundle.TypeKeyPage.String()}, page.Payload.ObjectTypes,
			"container keeps default Page rows — the plan is authoritative, no naive fallback per container")
	})

	t.Run("default construction plans with the naive planner", func(t *testing.T) {
		// when — no options: parity path
		sink := runScriptedWithOptions(t)

		// then — same verdict the suggestor produced before the plan phase
		suggested := issueMessages(sink, importv2.IssueTypeSuggested)
		require.Len(t, suggested, 1)
		assert.Contains(t, suggested[0], `database "Tasks" pages imported as task (container name)`)
		assert.Empty(t, issueMessages(sink, importv2.IssuePropertyMapped))
	})
}
