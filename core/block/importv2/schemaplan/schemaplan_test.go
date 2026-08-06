package schemaplan

import (
	"context"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func taskSchemas() []ContainerSchema {
	return []ContainerSchema{{
		Id:   "ds1",
		Name: "Sprint work",
		Properties: []PropertySchema{
			{Id: "p1", Name: "Deadline", Format: model.RelationFormat_date},
			{Id: "p2", Name: "State", Format: model.RelationFormat_status},
			{Id: "p3", Name: "Notes", Format: model.RelationFormat_longtext},
		},
	}}
}

func collectIssues(issues *[]importv2.Issue) func(importv2.Issue) {
	return func(issue importv2.Issue) { *issues = append(*issues, issue) }
}

func TestSanitize(t *testing.T) {
	t.Run("valid plan passes through", func(t *testing.T) {
		// given — the plan mints its own type; bundled targets stay legal for
		// the load-bearing relations
		plan := Plan{
			NewTypes: []TypeDefinition{{Key: "sprintTask", Name: "Sprint task"}},
			Containers: map[string]ContainerPlan{
				"ds1": {
					TypeKey: "sprintTask",
					Reason:  "LLM plan",
					Properties: map[string]PropertyPlan{
						"p1": {Key: bundle.RelationKeyDueDate},
					},
				},
			},
		}
		var issues []importv2.Issue
		want := Plan{
			NewTypes: []TypeDefinition{{Key: "sprintTask", Name: "Sprint task"}},
			Containers: map[string]ContainerPlan{
				"ds1": {
					TypeKey: "sprintTask",
					Reason:  "LLM plan",
					Properties: map[string]PropertyPlan{
						"p1": {Key: bundle.RelationKeyDueDate, Format: model.RelationFormat_date},
					},
				},
			},
		}

		// when
		got := Sanitize(plan, taskSchemas(), collectIssues(&issues))

		// then
		assert.Equal(t, want, got)
		assert.Empty(t, issues)
	})

	t.Run("zero plan sanitizes to itself", func(t *testing.T) {
		got := Sanitize(Plan{}, taskSchemas(), nil)
		assert.Equal(t, Plan{}, got)
	})

	t.Run("unknown container dropped", func(t *testing.T) {
		// given
		plan := Plan{Containers: map[string]ContainerPlan{
			"nope": {TypeKey: bundle.TypeKeyTask},
		}}
		var issues []importv2.Issue

		// when
		got := Sanitize(plan, taskSchemas(), collectIssues(&issues))

		// then
		assert.Empty(t, got.Containers)
		require.Len(t, issues, 1)
		assert.Equal(t, importv2.IssueLLMPlanEntryDropped, issues[0].Code)
	})

	t.Run("unknown type dropped, property plans survive", func(t *testing.T) {
		// given
		plan := Plan{Containers: map[string]ContainerPlan{
			"ds1": {
				TypeKey:    domain.TypeKey("hallucinated"),
				Properties: map[string]PropertyPlan{"p1": {Key: bundle.RelationKeyDueDate}},
			},
		}}
		var issues []importv2.Issue

		// when
		got := Sanitize(plan, taskSchemas(), collectIssues(&issues))

		// then
		require.Contains(t, got.Containers, "ds1")
		assert.Empty(t, got.Containers["ds1"].TypeKey)
		assert.Equal(t, PropertyPlan{Key: bundle.RelationKeyDueDate, Format: model.RelationFormat_date}, got.Containers["ds1"].Properties["p1"])
		require.Len(t, issues, 1)
	})

	t.Run("plan-defined type is a valid target", func(t *testing.T) {
		// given
		plan := Plan{
			NewTypes: []TypeDefinition{{
				Key: "sprint", Name: "Sprint", Layout: model.ObjectType_todo,
				Properties: []TypeProperty{{Key: bundle.RelationKeyDueDate, Featured: true}},
			}},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: "sprint"}},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then
		require.Len(t, got.NewTypes, 1)
		assert.Equal(t, domain.TypeKey("sprint"), got.Containers["ds1"].TypeKey)
	})

	t.Run("new type clashing a bundled key is re-keyed, never reusing the bundled type", func(t *testing.T) {
		// given — reusing the bundled key would reshape the built-in type
		// space-wide, so the plan gets its own key instead
		plan := Plan{
			NewTypes:   []TypeDefinition{{Key: bundle.TypeKeyTask, Name: "Task"}},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: bundle.TypeKeyTask}},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then
		require.Len(t, got.NewTypes, 1)
		assert.NotEqual(t, bundle.TypeKeyTask, got.NewTypes[0].Key)
		assert.Equal(t, got.NewTypes[0].Key, got.Containers["ds1"].TypeKey)
	})

	t.Run("denied target dropped", func(t *testing.T) {
		// given
		plan := Plan{Containers: map[string]ContainerPlan{
			"ds1": {Properties: map[string]PropertyPlan{
				"p3": {Key: bundle.RelationKeyName},
			}},
		}}
		var issues []importv2.Issue

		// when
		got := Sanitize(plan, taskSchemas(), collectIssues(&issues))

		// then
		assert.Empty(t, got.Containers)
		require.Len(t, issues, 1)
	})

	t.Run("illegal format change onto bundled relation dropped", func(t *testing.T) {
		// given: Notes is longtext, done is checkbox — not convertible
		plan := Plan{Containers: map[string]ContainerPlan{
			"ds1": {Properties: map[string]PropertyPlan{
				"p3": {Key: bundle.RelationKeyDone},
			}},
		}}
		var issues []importv2.Issue

		// when
		got := Sanitize(plan, taskSchemas(), collectIssues(&issues))

		// then
		assert.Empty(t, got.Containers)
		require.Len(t, issues, 1)
	})

	t.Run("bundled target strips name and format overrides", func(t *testing.T) {
		// given
		plan := Plan{Containers: map[string]ContainerPlan{
			"ds1": {Properties: map[string]PropertyPlan{
				"p1": {Key: bundle.RelationKeyDueDate, Name: "Custom", Format: model.RelationFormat_longtext},
			}},
		}}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then
		assert.Equal(t, PropertyPlan{Key: bundle.RelationKeyDueDate, Format: model.RelationFormat_date}, got.Containers["ds1"].Properties["p1"])
	})

	t.Run("custom target keeps overrides within the format family", func(t *testing.T) {
		// given
		plan := Plan{Containers: map[string]ContainerPlan{
			"ds1": {Properties: map[string]PropertyPlan{
				"p3": {Key: "meetingNotes", Name: "Meeting notes", Format: model.RelationFormat_shorttext},
			}},
		}}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then — the key is scoped to the container that declared it
		want := PropertyPlan{Key: ScopedKey("meetingNotes", "ds1"), Name: "Meeting notes", Format: model.RelationFormat_shorttext}
		assert.Equal(t, want, got.Containers["ds1"].Properties["p3"])
	})

	t.Run("system bundled relations are not allowed targets", func(t *testing.T) {
		// given — isArchived/isHidden are checkbox-format system relations a
		// checkbox source would otherwise legally remap onto
		plan := Plan{Containers: map[string]ContainerPlan{
			"ds1": {Properties: map[string]PropertyPlan{
				"p1": {Key: bundle.RelationKeyLastModifiedDate}, // date→date
				"p3": {Key: bundle.RelationKeyCoverId},          // text→text
			}},
		}}
		var issues []importv2.Issue

		// when
		got := Sanitize(plan, taskSchemas(), collectIssues(&issues))

		// then — the allow-list, not format legality, decides
		assert.Empty(t, got.Containers)
		require.Len(t, issues, 2)
		for _, issue := range issues {
			assert.Contains(t, issue.Message, "not an allowed plan target")
		}
	})

	t.Run("duplicate targets within a container keep the first source", func(t *testing.T) {
		// given — two properties onto dueDate would collide on page details
		schemas := []ContainerSchema{{
			Id: "ds1",
			Properties: []PropertySchema{
				{Id: "a", Name: "Deadline", Format: model.RelationFormat_date},
				{Id: "b", Name: "Finish by", Format: model.RelationFormat_date},
			},
		}}
		plan := Plan{Containers: map[string]ContainerPlan{
			"ds1": {Properties: map[string]PropertyPlan{
				"a": {Key: bundle.RelationKeyDueDate},
				"b": {Key: bundle.RelationKeyDueDate},
			}},
		}}
		var issues []importv2.Issue

		// when
		got := Sanitize(plan, schemas, collectIssues(&issues))

		// then — sorted order: "a" wins, "b" drops
		require.Len(t, got.Containers["ds1"].Properties, 1)
		assert.Contains(t, got.Containers["ds1"].Properties, "a")
		require.Len(t, issues, 1)
		assert.Contains(t, issues[0].Message, "duplicates target")
	})

	t.Run("custom target format anchors across containers of one type", func(t *testing.T) {
		// given — containers sharing a type share its properties, so their
		// formats must still agree: date first (sorted order), text second
		schemas := []ContainerSchema{
			{Id: "a", Properties: []PropertySchema{{Id: "p", Name: "When", Format: model.RelationFormat_date}}},
			{Id: "b", Properties: []PropertySchema{{Id: "p", Name: "When", Format: model.RelationFormat_longtext}}},
		}
		plan := Plan{
			NewTypes: []TypeDefinition{{Key: "event", Name: "Event"}},
			Containers: map[string]ContainerPlan{
				"a": {TypeKey: "event", Properties: map[string]PropertyPlan{"p": {Key: "when"}}},
				"b": {TypeKey: "event", Properties: map[string]PropertyPlan{"p": {Key: "when"}}},
			},
		}
		var issues []importv2.Issue

		// when
		got := Sanitize(plan, schemas, collectIssues(&issues))

		// then
		assert.Equal(t, model.RelationFormat_date, got.Containers["a"].Properties["p"].Format)
		assert.Empty(t, got.Containers["b"].Properties)
		require.Len(t, issues, 1)
		assert.Contains(t, issues[0].Message, "conflicts with target")
	})

	t.Run("type definition anchors the format, incompatible container entry drops", func(t *testing.T) {
		// given — the verified corruption scenario: a select property onto a
		// number-format type property
		schemas := []ContainerSchema{{
			Id: "ds1",
			Properties: []PropertySchema{
				{Id: "prio", Name: "Priority", Format: model.RelationFormat_status},
			},
		}}
		plan := Plan{
			NewTypes: []TypeDefinition{{
				Key: "sprint", Name: "Sprint",
				Properties: []TypeProperty{{Key: "effort", Format: model.RelationFormat_number}},
			}},
			Containers: map[string]ContainerPlan{
				"ds1": {TypeKey: "sprint", Properties: map[string]PropertyPlan{
					"prio": {Key: "effort"},
				}},
			},
		}
		var issues []importv2.Issue

		// when
		got := Sanitize(plan, schemas, collectIssues(&issues))

		// then — the entry drops; the type keeps its declared format
		assert.Empty(t, got.Containers["ds1"].Properties)
		assert.Equal(t, domain.TypeKey("sprint"), got.Containers["ds1"].TypeKey)
		require.Len(t, issues, 1)
	})

	t.Run("format-less type property inherits the containers' anchor", func(t *testing.T) {
		// given
		schemas := []ContainerSchema{{
			Id:         "ds1",
			Properties: []PropertySchema{{Id: "d", Name: "When", Format: model.RelationFormat_date}},
		}}
		plan := Plan{
			NewTypes: []TypeDefinition{{
				Key: "sprint", Name: "Sprint",
				Properties: []TypeProperty{{Key: "when"}}, // format 0
			}},
			Containers: map[string]ContainerPlan{
				"ds1": {TypeKey: "sprint", Properties: map[string]PropertyPlan{"d": {Key: "when"}}},
			},
		}

		// when
		got := Sanitize(plan, schemas, nil)

		// then
		require.Len(t, got.NewTypes, 1)
		assert.Equal(t, model.RelationFormat_date, got.NewTypes[0].Properties[0].Format)
		assert.Equal(t, model.RelationFormat_date, got.Containers["ds1"].Properties["d"].Format)
	})

	t.Run("dropped-entry issue order is deterministic", func(t *testing.T) {
		// given — many invalid entries across several containers
		plan := Plan{Containers: map[string]ContainerPlan{}}
		var schemas []ContainerSchema
		for _, id := range []string{"c1", "c2", "c3", "c4", "c5"} {
			schemas = append(schemas, ContainerSchema{Id: id, Properties: []PropertySchema{
				{Id: "x", Name: "X", Format: model.RelationFormat_longtext},
				{Id: "y", Name: "Y", Format: model.RelationFormat_longtext},
			}})
			plan.Containers[id] = ContainerPlan{Properties: map[string]PropertyPlan{
				"x": {Key: bundle.RelationKeyName},
				"y": {Key: bundle.RelationKeyId},
			}}
		}
		var first []string
		for run := 0; run < 20; run++ {
			var issues []importv2.Issue
			Sanitize(plan, schemas, collectIssues(&issues))
			var messages []string
			for _, issue := range issues {
				messages = append(messages, issue.SourceKey+": "+issue.Message)
			}
			if run == 0 {
				first = messages
				require.Len(t, first, 10)
				continue
			}
			require.Equal(t, first, messages, "issue order must not vary between runs")
		}
	})

	t.Run("status to tag allowed, date to checkbox not", func(t *testing.T) {
		assert.True(t, FormatChangeAllowed(model.RelationFormat_status, model.RelationFormat_tag))
		assert.True(t, FormatChangeAllowed(model.RelationFormat_email, model.RelationFormat_longtext))
		assert.False(t, FormatChangeAllowed(model.RelationFormat_date, model.RelationFormat_checkbox))
		assert.False(t, FormatChangeAllowed(model.RelationFormat_longtext, model.RelationFormat_status))
	})
}

func TestNaivePlanner(t *testing.T) {
	t.Run("wraps typesuggest verdicts per container", func(t *testing.T) {
		// given
		schemas := []ContainerSchema{
			{Id: "a", Name: "Tasks"},
			{Id: "b", Name: "Random stuff"},
			{Id: "c", Name: "CRM", Properties: []PropertySchema{
				{Id: "e", Name: "Email", Format: model.RelationFormat_email},
				{Id: "p", Name: "Phone", Format: model.RelationFormat_phone},
			}},
		}

		// when
		plan, err := NewNaive().Plan(context.Background(), schemas)

		// then
		require.NoError(t, err)
		assert.Equal(t, bundle.TypeKeyTask, plan.Containers["a"].TypeKey)
		assert.NotContains(t, plan.Containers, "b")
		assert.Equal(t, bundle.TypeKeyContact, plan.Containers["c"].TypeKey)
		assert.Equal(t, "email and phone properties", plan.Containers["c"].Reason)
	})

	t.Run("no verdicts yields the zero plan", func(t *testing.T) {
		plan, err := NewNaive().Plan(context.Background(), []ContainerSchema{{Id: "a", Name: "Misc"}})
		require.NoError(t, err)
		assert.Empty(t, plan.Containers)
	})
}

func TestCustomKeys(t *testing.T) {
	t.Run("deterministic and distinct per plan key", func(t *testing.T) {
		assert.Equal(t, CustomRelationKey("sprintGoal"), CustomRelationKey("sprintGoal"))
		assert.NotEqual(t, CustomRelationKey("sprintGoal"), CustomRelationKey("sprintgoal"))
		assert.Equal(t, CustomTypeKey("sprint"), CustomTypeKey("sprint"))
		assert.NotEqual(t, string(CustomRelationKey("sprint")), string(CustomTypeKey("sprint")))
	})
}

// twoCategorySchemas reproduces the shape that merged four databases' Category
// vocabularies into one option pool in the 2026-08-04 live import.
func twoCategorySchemas() []ContainerSchema {
	return []ContainerSchema{
		{Id: "recipes", Name: "Recipe SB", Properties: []PropertySchema{
			{Id: "c1", Name: "Category", Format: model.RelationFormat_status, Options: []string{"Breakfast", "Dinner"}},
			{Id: "r1", Name: "Meal Calendar", Format: model.RelationFormat_object},
		}},
		{Id: "launch", Name: "Launch Tracker", Properties: []PropertySchema{
			{Id: "c2", Name: "Category", Format: model.RelationFormat_status, Options: []string{"Marketing", "Sales"}},
			{Id: "r2", Name: "Project", Format: model.RelationFormat_object},
		}},
	}
}

func TestAlwaysMint(t *testing.T) {
	t.Run("same select target in two containers is scoped per container", func(t *testing.T) {
		// given — the model merges both Category properties onto one key
		plan := Plan{
			NewTypes: []TypeDefinition{
				{Key: "recipe", Name: "Recipe"},
				{Key: "launchItem", Name: "Launch item"},
			},
			Containers: map[string]ContainerPlan{
				"recipes": {TypeKey: "recipe", Properties: map[string]PropertyPlan{"c1": {Key: "category"}}},
				"launch":  {TypeKey: "launchItem", Properties: map[string]PropertyPlan{"c2": {Key: "category"}}},
			},
		}

		// when
		got := Sanitize(plan, twoCategorySchemas(), nil)

		// then — the vocabularies must land in two separate relations
		recipeKey := got.Containers["recipes"].Properties["c1"].Key
		launchKey := got.Containers["launch"].Properties["c2"].Key
		require.NotEmpty(t, recipeKey)
		require.NotEmpty(t, launchKey)
		assert.NotEqual(t, recipeKey, launchKey, "select vocabularies merged into one option pool")
		assert.NotEqual(t, CustomRelationKey(recipeKey), CustomRelationKey(launchKey))
	})

	t.Run("targets of every format are scoped, not just selects", func(t *testing.T) {
		// given — a property belongs to its type whatever its format; sharing
		// one across types means naming a whitelisted bundled target instead
		plan := Plan{
			NewTypes: []TypeDefinition{{Key: "recipe", Name: "Recipe"}, {Key: "launchItem", Name: "Launch item"}},
			Containers: map[string]ContainerPlan{
				"recipes": {TypeKey: "recipe", Properties: map[string]PropertyPlan{"r1": {Key: "project"}}},
				"launch":  {TypeKey: "launchItem", Properties: map[string]PropertyPlan{"r2": {Key: "project"}}},
			},
		}

		// when
		got := Sanitize(plan, twoCategorySchemas(), nil)

		// then
		assert.NotEqual(t, got.Containers["recipes"].Properties["r1"].Key,
			got.Containers["launch"].Properties["r2"].Key)
	})

	t.Run("containers sharing a type share its properties", func(t *testing.T) {
		// given — two containers the plan calls the same kind of thing
		plan := Plan{
			NewTypes: []TypeDefinition{{Key: "catalogued", Name: "Catalogued item"}},
			Containers: map[string]ContainerPlan{
				"recipes": {TypeKey: "catalogued", Properties: map[string]PropertyPlan{"c1": {Key: "category"}}},
				"launch":  {TypeKey: "catalogued", Properties: map[string]PropertyPlan{"c2": {Key: "category"}}},
			},
		}

		// when
		got := Sanitize(plan, twoCategorySchemas(), nil)

		// then — one type, one property, one vocabulary
		assert.Equal(t, got.Containers["recipes"].Properties["c1"].Key,
			got.Containers["launch"].Properties["c2"].Key)
	})

	t.Run("whitelisted bundled targets stay shared across types", func(t *testing.T) {
		// given — the escape hatch: naming a bundled target is how a plan asks
		// for one property across every type
		plan := Plan{
			NewTypes: []TypeDefinition{{Key: "recipe", Name: "Recipe"}, {Key: "launchItem", Name: "Launch item"}},
			Containers: map[string]ContainerPlan{
				"recipes": {TypeKey: "recipe", Properties: map[string]PropertyPlan{"c1": {Key: bundle.RelationKeyTag}}},
				"launch":  {TypeKey: "launchItem", Properties: map[string]PropertyPlan{"c2": {Key: bundle.RelationKeyTag}}},
			},
		}

		// when
		got := Sanitize(plan, twoCategorySchemas(), nil)

		// then
		assert.Equal(t, bundle.RelationKeyTag, got.Containers["recipes"].Properties["c1"].Key)
		assert.Equal(t, bundle.RelationKeyTag, got.Containers["launch"].Properties["c2"].Key)
	})

	t.Run("a type definition agrees with its container on the scoped key", func(t *testing.T) {
		// given — the type declares the same select the container remaps
		plan := Plan{
			NewTypes: []TypeDefinition{{
				Key: "recipe", Name: "Recipe",
				Properties: []TypeProperty{{Key: "category", Name: "Category", Format: model.RelationFormat_status}},
			}},
			Containers: map[string]ContainerPlan{
				"recipes": {TypeKey: "recipe", Properties: map[string]PropertyPlan{"c1": {Key: "category"}}},
			},
		}

		// when
		got := Sanitize(plan, twoCategorySchemas(), nil)

		// then — or the type's recommended relation points at a relation nobody emits
		require.Len(t, got.NewTypes, 1)
		require.Len(t, got.NewTypes[0].Properties, 1)
		assert.Equal(t, got.Containers["recipes"].Properties["c1"].Key, got.NewTypes[0].Properties[0].Key)
	})

	t.Run("pointing a container at a bundled type stays legal for the naive planner", func(t *testing.T) {
		// given — every typesuggest verdict is a bundled type key, and naming
		// one as a page's type changes nothing about the bundled type itself
		plan := Plan{Containers: map[string]ContainerPlan{
			"ds1": {TypeKey: bundle.TypeKeyTask, Reason: "container name"},
		}}
		var issues []importv2.Issue

		// when
		got := Sanitize(plan, taskSchemas(), collectIssues(&issues))

		// then
		assert.Equal(t, bundle.TypeKeyTask, got.Containers["ds1"].TypeKey)
		assert.Empty(t, issues)
	})

	t.Run("type definition colliding with a bundled key is re-keyed, container follows", func(t *testing.T) {
		// given — the model spells its minted type "task"
		plan := Plan{
			NewTypes: []TypeDefinition{{Key: bundle.TypeKeyTask, Name: "Sprint task"}},
			Containers: map[string]ContainerPlan{
				"ds1": {TypeKey: bundle.TypeKeyTask, Reason: "LLM plan"},
			},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then — the container keeps a working minted type instead of being lost
		require.Len(t, got.NewTypes, 1)
		assert.NotEqual(t, bundle.TypeKeyTask, got.NewTypes[0].Key)
		require.Contains(t, got.Containers, "ds1")
		assert.Equal(t, got.NewTypes[0].Key, got.Containers["ds1"].TypeKey)
	})

	t.Run("dropped allowlist targets are rejected", func(t *testing.T) {
		for _, key := range []domain.RelationKey{
			bundle.RelationKeyAssignee, bundle.RelationKeyAuthor,
			bundle.RelationKeyCompany, bundle.RelationKeyPriority,
		} {
			t.Run(key.String(), func(t *testing.T) {
				// given
				plan := Plan{
					NewTypes: []TypeDefinition{{Key: "sprint", Name: "Sprint"}},
					Containers: map[string]ContainerPlan{
						"ds1": {TypeKey: "sprint", Properties: map[string]PropertyPlan{"p2": {Key: key}}},
					},
				}
				var issues []importv2.Issue

				// when
				got := Sanitize(plan, taskSchemas(), collectIssues(&issues))

				// then
				assert.Empty(t, got.Containers["ds1"].Properties)
				require.NotEmpty(t, issues)
				assert.Equal(t, importv2.IssueLLMPlanEntryDropped, issues[0].Code)
			})
		}
	})

	t.Run("prose in a relation name falls back to the source property name", func(t *testing.T) {
		// given — both live models wrote explanations into the name field
		plan := Plan{
			NewTypes: []TypeDefinition{{Key: "sprint", Name: "Sprint"}},
			Containers: map[string]ContainerPlan{
				"ds1": {TypeKey: "sprint", Properties: map[string]PropertyPlan{
					"p2": {Key: "state", Name: "State (as state) from Sprint work mapped to a per-container select property."},
				}},
			},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then
		assert.Equal(t, "State", got.Containers["ds1"].Properties["p2"].Name)
	})

	t.Run("control characters are stripped from a type name", func(t *testing.T) {
		// given
		plan := Plan{
			NewTypes:   []TypeDefinition{{Key: "sprint", Name: "Sprint\n\tTask"}},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: "sprint"}},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then
		require.Len(t, got.NewTypes, 1)
		assert.Equal(t, "Sprint Task", got.NewTypes[0].Name)
	})

	t.Run("an icon outside the vocabulary is dropped", func(t *testing.T) {
		// given
		plan := Plan{
			NewTypes:   []TypeDefinition{{Key: "sprint", Name: "Sprint", IconName: "not-a-real-icon"}},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: "sprint"}},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then
		require.Len(t, got.NewTypes, 1)
		assert.Empty(t, got.NewTypes[0].IconName)
	})

	t.Run("an icon inside the vocabulary survives", func(t *testing.T) {
		// given
		plan := Plan{
			NewTypes:   []TypeDefinition{{Key: "sprint", Name: "Sprint", IconName: "checkbox", PluralName: "Sprints"}},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: "sprint"}},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then
		require.Len(t, got.NewTypes, 1)
		assert.Equal(t, "checkbox", got.NewTypes[0].IconName)
		assert.Equal(t, "Sprints", got.NewTypes[0].PluralName)
	})
}

func TestTypeObject(t *testing.T) {
	t.Run("carries plural name and icon", func(t *testing.T) {
		// given — minting is now the only path, so a type must not look
		// unfinished next to a bundled one
		def := TypeDefinition{
			Key: "sprintTask", Name: "Sprint task", PluralName: "Sprint tasks",
			IconName: "checkbox", Layout: model.ObjectType_todo,
		}

		// when
		object, _, err := TypeObject(def)

		// then
		require.NoError(t, err)
		assert.Equal(t, "Sprint task", object.Payload.Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "Sprint tasks", object.Payload.Details.GetString(bundle.RelationKeyPluralName))
		assert.Equal(t, "checkbox", object.Payload.Details.GetString(bundle.RelationKeyIconName))
	})

	t.Run("omits plural name and icon when the plan gave none", func(t *testing.T) {
		// given
		def := TypeDefinition{Key: "sprintTask", Name: "Sprint task"}

		// when
		object, _, err := TypeObject(def)

		// then
		require.NoError(t, err)
		assert.False(t, object.Payload.Details.Has(bundle.RelationKeyPluralName))
		assert.False(t, object.Payload.Details.Has(bundle.RelationKeyIconName))
	})
}

func TestSharedTypeAcrossContainers(t *testing.T) {
	t.Run("a type definition and containers sharing it agree on the scoped key", func(t *testing.T) {
		// given — two databases that are the same kind of thing: they become
		// two collections over one type, not two types
		plan := Plan{
			NewTypes: []TypeDefinition{{
				Key: "catalogued", Name: "Catalogued item",
				Properties: []TypeProperty{{Key: "category", Name: "Category", Format: model.RelationFormat_status}},
			}},
			Containers: map[string]ContainerPlan{
				"recipes": {TypeKey: "catalogued", Properties: map[string]PropertyPlan{"c1": {Key: "category"}}},
				"launch":  {TypeKey: "catalogued", Properties: map[string]PropertyPlan{"c2": {Key: "category"}}},
			},
		}

		// when
		got := Sanitize(plan, twoCategorySchemas(), nil)

		// then — one relation, named by the type, referenced by both
		// containers AND by the type's own recommended relations
		require.Len(t, got.NewTypes, 1)
		require.Len(t, got.NewTypes[0].Properties, 1)
		shared := got.Containers["recipes"].Properties["c1"].Key
		assert.Equal(t, shared, got.Containers["launch"].Properties["c2"].Key)
		assert.Equal(t, shared, got.NewTypes[0].Properties[0].Key,
			"the type's recommended relation must be the one its containers write to")
	})
}

// TestReviewFindings pins the defects a four-lens review of 2026-08-06 found
// in the always-mint work. Each subtest failed before its fix.
func TestReviewFindings(t *testing.T) {
	t.Run("a dropped type does not lend its scope to two containers", func(t *testing.T) {
		// given — the model types both containers "catalogued" but never
		// defines it, so neither ends up typed
		plan := Plan{Containers: map[string]ContainerPlan{
			"recipes": {TypeKey: "catalogued", Properties: map[string]PropertyPlan{"c1": {Key: "category"}}},
			"launch":  {TypeKey: "catalogued", Properties: map[string]PropertyPlan{"c2": {Key: "category"}}},
		}}

		// when
		got := Sanitize(plan, twoCategorySchemas(), nil)

		// then — an unrealised kind must not still merge their option pools
		assert.NotEqual(t, got.Containers["recipes"].Properties["c1"].Key,
			got.Containers["launch"].Properties["c2"].Key)
	})

	t.Run("a type key equal to another container's id does not collide scopes", func(t *testing.T) {
		// given — ids and type keys share one string namespace
		plan := Plan{Containers: map[string]ContainerPlan{
			"recipes": {TypeKey: "launch", Properties: map[string]PropertyPlan{"c1": {Key: "category"}}},
			"launch":  {Properties: map[string]PropertyPlan{"c2": {Key: "category"}}},
		}}

		// when
		got := Sanitize(plan, twoCategorySchemas(), nil)

		// then
		assert.NotEqual(t, got.Containers["recipes"].Properties["c1"].Key,
			got.Containers["launch"].Properties["c2"].Key)
	})

	t.Run("ScopedKey cannot be forged by an @ in the key or scope", func(t *testing.T) {
		assert.NotEqual(t, ScopedKey("a", "b@c"), ScopedKey("a@b", "c"))
		assert.NotEqual(t, CustomRelationKey(ScopedKey("a", "b@c")), CustomRelationKey(ScopedKey("a@b", "c")))
	})

	t.Run("a type property name is bounded like every other plan-supplied name", func(t *testing.T) {
		// given — both live models write explanations into name fields, and
		// the type definition emits the relation FIRST, so its name wins
		prose := "State (as workState) from Sprint work mapped to a per-container select property."
		plan := Plan{
			NewTypes: []TypeDefinition{{
				Key: "sprint", Name: "Sprint",
				Properties: []TypeProperty{{Key: "workState", Name: prose, Format: model.RelationFormat_status}},
			}},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: "sprint"}},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then
		require.Len(t, got.NewTypes, 1)
		require.Len(t, got.NewTypes[0].Properties, 1)
		assert.NotEqual(t, prose, got.NewTypes[0].Properties[0].Name)
		assert.LessOrEqual(t, len([]rune(got.NewTypes[0].Properties[0].Name)), 64)
	})

	t.Run("a nameless type property never exposes the internal scoped key", func(t *testing.T) {
		// given — the wire schema requires the field but "" is legal
		plan := Plan{
			NewTypes: []TypeDefinition{{
				Key: "sprint", Name: "Sprint",
				Properties: []TypeProperty{{Key: "effort", Format: model.RelationFormat_number}},
			}},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: "sprint"}},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then — "effort@sprint" must never reach a user-visible name
		require.Len(t, got.NewTypes, 1)
		require.Len(t, got.NewTypes[0].Properties, 1)
		assert.Equal(t, "effort", got.NewTypes[0].Properties[0].Name)
	})

	t.Run("control characters are stripped from names", func(t *testing.T) {
		for _, hostile := range []struct{ name, in string }{
			{"NUL", "Spr\x00int"},
			{"escape", "Spr\x1b[31mint"},
			{"rtl override", "Sprint‮gnp.exe"},
		} {
			t.Run(hostile.name, func(t *testing.T) {
				// given
				plan := Plan{
					NewTypes:   []TypeDefinition{{Key: "sprint", Name: hostile.in}},
					Containers: map[string]ContainerPlan{"ds1": {TypeKey: "sprint"}},
				}

				// when
				got := Sanitize(plan, taskSchemas(), nil)

				// then
				require.Len(t, got.NewTypes, 1)
				for _, r := range got.NewTypes[0].Name {
					assert.False(t, unicode.IsControl(r) || unicode.Is(unicode.Cf, r),
						"control character %U survived into a display name", r)
				}
			})
		}
	})

	t.Run("an over-long name on a shared type keeps the type", func(t *testing.T) {
		// given — several containers sharing one type is the first-class case,
		// and then no single container's name is available as a fallback
		plan := Plan{
			NewTypes: []TypeDefinition{{Key: "catalogued", Name: strings.Repeat("prose ", 20)}},
			Containers: map[string]ContainerPlan{
				"recipes": {TypeKey: "catalogued"},
				"launch":  {TypeKey: "catalogued"},
			},
		}

		// when
		got := Sanitize(plan, twoCategorySchemas(), nil)

		// then — always-mint must not degrade to untyped pages here
		require.Len(t, got.NewTypes, 1, "the shared type was dropped for want of a name")
		assert.NotEmpty(t, got.NewTypes[0].Name)
		assert.Equal(t, got.NewTypes[0].Key, got.Containers["recipes"].TypeKey)
	})

	t.Run("a duplicate definition does not retype containers onto a decoy", func(t *testing.T) {
		// given — "task" re-keys to "plan_task", which a decoy already holds
		plan := Plan{
			NewTypes: []TypeDefinition{
				{Key: "plan_task", Name: "Decoy"},
				{Key: bundle.TypeKeyTask, Name: "Real"},
			},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: bundle.TypeKeyTask}},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then — the container must not silently inherit the decoy's shape
		if typeKey := got.Containers["ds1"].TypeKey; typeKey != "" {
			for _, def := range got.NewTypes {
				if def.Key == typeKey {
					assert.NotEqual(t, "Decoy", def.Name,
						"container typed onto a definition it never named")
				}
			}
		}
	})

	t.Run("duplicate property keys within one type definition collapse", func(t *testing.T) {
		// given
		plan := Plan{
			NewTypes: []TypeDefinition{{
				Key: "sprint", Name: "Sprint",
				Properties: []TypeProperty{
					{Key: "state", Name: "State", Format: model.RelationFormat_tag, Featured: true},
					{Key: "state", Name: "State again", Format: model.RelationFormat_date},
				},
			}},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: "sprint"}},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then — one relation must not be both featured and regular, nor carry
		// two declared formats
		require.Len(t, got.NewTypes, 1)
		assert.Len(t, got.NewTypes[0].Properties, 1)
	})
}
