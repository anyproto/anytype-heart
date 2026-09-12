package schemaplan

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
)

// Resolve is the plan phase's single reuse rule, shared by both converters
// (the three-review-rounds lesson: one rule, one implementation, or a
// sibling drifts).

func testSchemas() []ContainerSchema {
	return []ContainerSchema{{Id: "db1", Name: "Tasks"}}
}

func TestResolveReuse(t *testing.T) {
	noIssue := func(i importv2.Issue) {}

	t.Run("a preset plan short-circuits everything: no planner call, no re-sanitize", func(t *testing.T) {
		// given: a preset carrying an entry Sanitize would normally inspect
		preset := Plan{Containers: map[string]ContainerPlan{
			"db1": {TypeKey: domain.TypeKey("task"), Reason: "recorded"},
		}}
		poison := PlannerFunc(func(context.Context, []ContainerSchema) (Plan, error) {
			t.Error("the planner must never run under a preset")
			return Plan{}, nil
		})

		// when
		plan, err := Resolve(context.Background(), Reuse{Preset: &preset}, poison, testSchemas(),
			func(i importv2.Issue) { t.Errorf("no issue may be emitted under a preset: %v", i) })

		// then
		require.NoError(t, err)
		assert.Equal(t, preset, plan)
	})

	t.Run("a fresh run records the SANITIZED plan", func(t *testing.T) {
		// given: a planner returning an entry sanitize must drop (a container
		// the schemas do not name) next to a good one
		planner := PlannerFunc(func(context.Context, []ContainerSchema) (Plan, error) {
			return Plan{Containers: map[string]ContainerPlan{
				"db1":     {TypeKey: domain.TypeKey("task"), Reason: "planner"},
				"unknown": {TypeKey: domain.TypeKey("note"), Reason: "hallucinated"},
			}}, nil
		})
		var recorded *Plan

		// when
		plan, err := Resolve(context.Background(), Reuse{Record: func(p Plan) error {
			recorded = &p
			return nil
		}}, planner, testSchemas(), noIssue)

		// then: the record is exactly the plan the run converts under —
		// replaying it must reproduce the run, so raw planner output (which
		// re-sanitizing could treat differently) is never what lands
		require.NoError(t, err)
		require.NotNil(t, recorded)
		assert.Equal(t, plan, *recorded)
		assert.NotContains(t, recorded.Containers, "unknown",
			"the recorded plan must be the sanitized one")
	})

	t.Run("a record failure is fatal store trouble, not a silent skip", func(t *testing.T) {
		// given — the rule: a run that cannot journal must not keep going;
		// objects spooled under an unrecorded plan could never be resumed
		// consistently.
		planner := PlannerFunc(func(context.Context, []ContainerSchema) (Plan, error) {
			return Plan{}, nil
		})

		// when
		_, err := Resolve(context.Background(), Reuse{Record: func(Plan) error {
			return errors.New("disk full")
		}}, planner, testSchemas(), noIssue)

		// then
		require.Error(t, err)
		issue := importv2.AsIssue(err, importv2.SeverityObjectError, importv2.IssueObjectFailed)
		assert.Equal(t, importv2.SeverityFatal, issue.Severity)
		assert.Equal(t, importv2.IssueStoreError, issue.Code,
			"the failure must classify as OUR storage, not the user's source")
	})

	t.Run("planner failure still degrades to naive with the loud warning", func(t *testing.T) {
		// given
		planner := PlannerFunc(func(context.Context, []ContainerSchema) (Plan, error) {
			return Plan{}, errors.New("llm unreachable")
		})
		var issues []importv2.Issue

		// when
		_, err := Resolve(context.Background(), Reuse{}, planner, testSchemas(),
			func(i importv2.Issue) { issues = append(issues, i) })

		// then
		require.NoError(t, err)
		require.NotEmpty(t, issues)
		assert.Equal(t, importv2.IssueLLMPlanFailed, issues[0].Code)
	})
}
