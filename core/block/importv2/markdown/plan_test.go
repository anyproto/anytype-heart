package markdown

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// vaultFiles is a small Obsidian-shaped vault: one homogeneous folder with
// front matter, one root note.
var vaultFiles = map[string]string{
	".obsidian/app.json": "{}",
	"Work/alpha.md":      "---\nDeadline: 2024-03-05\nState: Doing\n---\n# Alpha\n\nBody.\n",
	"Work/beta.md":       "---\nDeadline: 2024-04-01\nState: Done\nEffort: 3\n---\n# Beta\n\nBody.\n",
	"note.md":            "# Note\n\nLoose.\n",
}

// planIssueMessages renders an issue the way the report does: the sentence,
// and the subject it is about.
func planIssueMessages(sink *recordingSink, code importv2.IssueCode) []string {
	var out []string
	for _, issue := range sink.issues {
		if issue.Code != code {
			continue
		}
		message := issue.Message
		if issue.Subject != "" {
			message += " — " + issue.Subject
		}
		out = append(out, message)
	}
	return out
}

func TestFolderPlan(t *testing.T) {
	t.Run("folder sweep feeds the planner union schema with options", func(t *testing.T) {
		// given
		var seen []schemaplan.ContainerSchema
		planner := schemaplan.PlannerFunc(func(_ context.Context, schemas []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			seen = schemas
			return schemaplan.Plan{}, nil
		})

		// when
		runConverterWithParams(t, vaultFiles, Params{Flavour: FlavourObsidian, Planner: planner})

		// then — one container for Work/, none for the heterogeneous root
		require.Len(t, seen, 1)
		assert.Equal(t, "dir:Work", seen[0].Id)
		assert.Equal(t, "Work", seen[0].Name)
		names := map[string]schemaplan.PropertySchema{}
		for _, property := range seen[0].Properties {
			names[property.Name] = property
		}
		require.Contains(t, names, "Deadline")
		require.Contains(t, names, "State")
		require.Contains(t, names, "Effort")
		assert.Equal(t, model.RelationFormat_date, names["Deadline"].Format)
		assert.ElementsMatch(t, []string{"Doing", "Done"}, names["State"].Options)
	})

	t.Run("plan types the folder and remaps a property onto a bundled relation", func(t *testing.T) {
		// given
		planner := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			return schemaplan.Plan{Containers: map[string]schemaplan.ContainerPlan{
				"dir:Work": {
					TypeKey: bundle.TypeKeyTask,
					Reason:  "LLM plan",
					Properties: map[string]schemaplan.PropertyPlan{
						"Deadline": {Key: bundle.RelationKeyDueDate},
					},
				},
			}}, nil
		})

		// when
		sink, _ := runConverterWithParams(t, vaultFiles, Params{Flavour: FlavourObsidian, Planner: planner})

		// then — pages typed, property landed on the bundled key
		alpha := sink.byKey("Work/alpha.md")
		require.NotNil(t, alpha)
		assert.Equal(t, []string{bundle.TypeKeyTask.String()}, alpha.Payload.ObjectTypes)
		assert.True(t, alpha.Payload.Details.Has(bundle.RelationKeyDueDate),
			"Deadline value lands under dueDate")
		assert.Nil(t, sink.byKey("relation:"+stableKey("md", "Deadline")),
			"no custom relation minted for the remapped property")

		loose := sink.byKey("note.md")
		require.NotNil(t, loose)
		assert.Equal(t, []string{bundle.TypeKeyPage.String()}, loose.Payload.ObjectTypes,
			"root pages are outside any container")

		suggested := planIssueMessages(sink, importv2.IssueTypeSuggested)
		require.Len(t, suggested, 1)
		assert.Contains(t, suggested[0], `Work → task (LLM plan)`)
		mapped := planIssueMessages(sink, importv2.IssuePropertyMapped)
		require.Len(t, mapped, 1, "one propertyMapped issue per folder+property, not per page")
		assert.Contains(t, mapped[0], `property "Deadline" imported as "Due date" (dueDate)`)
	})

	t.Run("custom shared key merges the property across pages and feeds the new type", func(t *testing.T) {
		// given — a plan-defined type whose property is also the remap target
		planner := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			return schemaplan.Plan{
				NewTypes: []schemaplan.TypeDefinition{{
					Key: "workItem", Name: "Work item", Layout: model.ObjectType_todo,
					Properties: []schemaplan.TypeProperty{
						{Key: "workState", Name: "State", Format: model.RelationFormat_status, Featured: true},
					},
				}},
				Containers: map[string]schemaplan.ContainerPlan{
					"dir:Work": {
						TypeKey: "workItem",
						Reason:  "LLM plan",
						Properties: map[string]schemaplan.PropertyPlan{
							"State": {Key: "workState", Format: model.RelationFormat_status},
						},
					},
				},
			}, nil
		})

		// when
		sink, _ := runConverterWithParams(t, vaultFiles, Params{Flavour: FlavourObsidian, Planner: planner})

		// then — the type object exists and pages carry its minted key
		typeObject := sink.byKey(schemaplan.TypeSourceKey("workItem"))
		require.NotNil(t, typeObject)
		assert.Equal(t, "Work item", typeObject.Payload.Details.GetString(bundle.RelationKeyName))
		minted := schemaplan.CustomTypeKey("workItem")
		alpha := sink.byKey("Work/alpha.md")
		require.NotNil(t, alpha)
		assert.Equal(t, []string{minted.String()}, alpha.Payload.ObjectTypes)

		// the relation exists once for the type; both pages' values live under
		// it. The key is scoped to the type, so a "State" select belonging to
		// another type cannot merge its options into this one.
		stateKey := schemaplan.CustomRelationKey(schemaplan.ScopedKey("workState", "workItem")).String()
		relation := sink.byKey("relation:" + stateKey)
		require.NotNil(t, relation)
		assert.Equal(t, "State", relation.Payload.Details.GetString(bundle.RelationKeyName))
		beta := sink.byKey("Work/beta.md")
		require.NotNil(t, beta)
		assert.True(t, beta.Payload.Details.Has(domain.RelationKey(stateKey)))
		// option values resolved under the shared key during parsing
		assert.NotNil(t, sink.byKey("option:"+stateKey+":Done"))
	})

	t.Run("page value that does not fit the target reverts per page", func(t *testing.T) {
		// given — Effort is a number on one page and prose on another; the
		// plan (validated against the sweep's number verdict) targets a minted
		// number-format relation
		files := map[string]string{
			".obsidian/app.json": "{}",
			"Work/a.md":          "---\nEffort: 3\n---\n# A\n\nBody.\n",
			"Work/b.md":          "---\nEffort: three days\n---\n# B\n\nBody.\n",
		}
		planner := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			return schemaplan.Plan{Containers: map[string]schemaplan.ContainerPlan{
				"dir:Work": {Properties: map[string]schemaplan.PropertyPlan{
					"Effort": {Key: "effortPoints", Name: "Effort points", Format: model.RelationFormat_number},
				}},
			}}, nil
		})

		// when
		sink, _ := runConverterWithParams(t, files, Params{Flavour: FlavourObsidian, Planner: planner})

		// then — the numeric page follows the remap, the prose page reverts
		effortKey := domain.RelationKey(schemaplan.CustomRelationKey(schemaplan.ScopedKey("effortPoints", "dir:Work")).String())
		a := sink.byKey("Work/a.md")
		require.NotNil(t, a)
		assert.True(t, a.Payload.Details.Has(effortKey))

		b := sink.byKey("Work/b.md")
		require.NotNil(t, b)
		assert.False(t, b.Payload.Details.Has(effortKey),
			"a string must not land in a number relation")
		mdKey := domain.RelationKey(stableKey("md", "Effort"))
		assert.Equal(t, "three days", b.Payload.Details.GetString(mdKey),
			"the value survives under the original md property")

		dropped := planIssueMessages(sink, importv2.IssueLLMPlanEntryDropped)
		require.Len(t, dropped, 1)
		assert.Contains(t, dropped[0], `values that do not fit`)
	})

	t.Run("planner failure degrades to naive with a warning", func(t *testing.T) {
		// given — folder named Tasks so the naive fallback has a verdict
		files := map[string]string{
			".obsidian/app.json": "{}",
			"Tasks/a.md":         "# A\n\nBody.\n",
		}
		planner := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			return schemaplan.Plan{}, errors.New("unreachable")
		})

		// when
		sink, _ := runConverterWithParams(t, files, Params{Flavour: FlavourObsidian, Planner: planner})

		// then
		require.Len(t, planIssueMessages(sink, importv2.IssueLLMPlanFailed), 1)
		suggested := planIssueMessages(sink, importv2.IssueTypeSuggested)
		require.Len(t, suggested, 1)
		assert.Contains(t, suggested[0], `Tasks → task (container name)`)
	})

	t.Run("explicit front-matter type beats the plan type, property remap still applies", func(t *testing.T) {
		// given
		files := map[string]string{
			".obsidian/app.json": "{}",
			"Work/typed.md":      "---\ntype: Meeting\nDeadline: 2024-03-05\n---\n# Typed\n\nBody.\n",
		}
		planner := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			return schemaplan.Plan{Containers: map[string]schemaplan.ContainerPlan{
				"dir:Work": {
					TypeKey:    bundle.TypeKeyTask,
					Reason:     "LLM plan",
					Properties: map[string]schemaplan.PropertyPlan{"Deadline": {Key: bundle.RelationKeyDueDate}},
				},
			}}, nil
		})

		// when
		sink, _ := runConverterWithParams(t, files, Params{Flavour: FlavourObsidian, Planner: planner})

		// then
		page := sink.byKey("Work/typed.md")
		require.NotNil(t, page)
		assert.NotEqual(t, []string{bundle.TypeKeyTask.String()}, page.Payload.ObjectTypes,
			"explicit type wins over the plan")
		assert.True(t, page.Payload.Details.Has(bundle.RelationKeyDueDate),
			"property remap applies regardless of type origin")
	})

	t.Run("generic flavour has no containers and no plan behavior", func(t *testing.T) {
		// given
		called := false
		planner := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			called = true
			return schemaplan.Plan{}, nil
		})
		files := map[string]string{"Tasks/a.md": "# A\n\nBody.\n"}

		// when
		sink, _ := runConverterWithParams(t, files, Params{Planner: planner})

		// then
		assert.False(t, called, "no containers → planner never invoked")
		assert.Empty(t, planIssueMessages(sink, importv2.IssueTypeSuggested))
	})
}
