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
				TypeKey:    bundle.TypeKeyTask,
				Reason:     "LLM plan",
				Properties: map[string]PropertyPlan{"p1": {Key: bundle.RelationKeyDueDate}},
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
		assert.Equal(t, PropertyPlan{Key: bundle.RelationKeyDueDate}, got.Containers["ds1"].Properties["p1"])
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
		assert.Equal(t, PropertyPlan{Key: bundle.RelationKeyDueDate}, got.Containers["ds1"].Properties["p1"])
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

	t.Run("status to tag allowed, date to checkbox not", func(t *testing.T) {
		assert.True(t, formatChangeAllowed(model.RelationFormat_status, model.RelationFormat_tag))
		assert.True(t, formatChangeAllowed(model.RelationFormat_email, model.RelationFormat_longtext))
		assert.False(t, formatChangeAllowed(model.RelationFormat_date, model.RelationFormat_checkbox))
		assert.False(t, formatChangeAllowed(model.RelationFormat_longtext, model.RelationFormat_status))
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
