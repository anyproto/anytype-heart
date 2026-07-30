package schemaplan

import (
	"context"
	"testing"

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
		// given
		plan := Plan{Containers: map[string]ContainerPlan{
			"ds1": {
				TypeKey: bundle.TypeKeyTask,
				Reason:  "LLM plan",
				Properties: map[string]PropertyPlan{
					"p1": {Key: bundle.RelationKeyDueDate},
				},
			},
		}}
		var issues []importv2.Issue
		want := Plan{Containers: map[string]ContainerPlan{
			"ds1": {
				TypeKey: bundle.TypeKeyTask,
				Reason:  "LLM plan",
				Properties: map[string]PropertyPlan{
					"p1": {Key: bundle.RelationKeyDueDate, Format: model.RelationFormat_date},
				},
			},
		}}

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

	t.Run("new type clashing a bundled key collapses onto the bundled type", func(t *testing.T) {
		// given
		plan := Plan{
			NewTypes:   []TypeDefinition{{Key: bundle.TypeKeyTask, Name: "Task"}},
			Containers: map[string]ContainerPlan{"ds1": {TypeKey: bundle.TypeKeyTask}},
		}

		// when
		got := Sanitize(plan, taskSchemas(), nil)

		// then
		assert.Empty(t, got.NewTypes)
		assert.Equal(t, bundle.TypeKeyTask, got.Containers["ds1"].TypeKey)
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

		// then
		want := PropertyPlan{Key: "meetingNotes", Name: "Meeting notes", Format: model.RelationFormat_shorttext}
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

	t.Run("custom target format anchors across containers", func(t *testing.T) {
		// given — two containers merge onto one key: date first (sorted
		// order), text second — incompatible with the settled anchor
		schemas := []ContainerSchema{
			{Id: "a", Properties: []PropertySchema{{Id: "p", Name: "When", Format: model.RelationFormat_date}}},
			{Id: "b", Properties: []PropertySchema{{Id: "p", Name: "When", Format: model.RelationFormat_longtext}}},
		}
		plan := Plan{Containers: map[string]ContainerPlan{
			"a": {Properties: map[string]PropertyPlan{"p": {Key: "when"}}},
			"b": {Properties: map[string]PropertyPlan{"p": {Key: "when"}}},
		}}
		var issues []importv2.Issue

		// when
		got := Sanitize(plan, schemas, collectIssues(&issues))

		// then
		assert.Equal(t, model.RelationFormat_date, got.Containers["a"].Properties["p"].Format)
		assert.NotContains(t, got.Containers, "b")
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
