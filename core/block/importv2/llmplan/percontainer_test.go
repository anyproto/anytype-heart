package llmplan

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var perContainerSchemas = []schemaplan.ContainerSchema{
	{
		Id:   "ds1",
		Name: "Chores",
		Properties: []schemaplan.PropertySchema{
			{Id: "p1", Name: "Done?", Format: model.RelationFormat_checkbox},
		},
	},
	{
		Id:   "ds2",
		Name: "Sprint chores",
		Properties: []schemaplan.PropertySchema{
			{Id: "q1", Name: "Done?", Format: model.RelationFormat_checkbox},
		},
	},
	{
		Id:   "ds3",
		Name: "Recipes",
		Properties: []schemaplan.PropertySchema{
			{Id: "r1", Name: "Servings", Format: model.RelationFormat_number},
		},
	},
}

func TestPlanPerContainer(t *testing.T) {
	t.Run("normalize-equal spellings collapse into one kind", func(t *testing.T) {
		// given — one call per container in evidence order (ds1, ds2, ds3);
		// "Chore" and "chore " normalize alike, the recipe stays its own kind
		fake := newFakeLLM(t,
			fakeReply{content: `{"kind": "Chore"}`},
			fakeReply{content: `{"kind": "chore "}`},
			fakeReply{content: `{"kind": "Recipe"}`},
		)
		planner := newTestPlanner(t, fake, WithPerContainerCalls())

		// when
		got, err := planner.Plan(context.Background(), perContainerSchemas)

		// then — the first spelling wins the kind name; the completion
		// checkbox makes the chore kind a todo, the recipe kind stays basic
		require.NoError(t, err)
		require.Len(t, fake.requests, 3)
		require.Len(t, got.NewTypes, 2)
		assert.Equal(t, "Chore", got.NewTypes[0].Name)
		assert.Equal(t, model.ObjectType_todo, got.NewTypes[0].Layout)
		assert.Equal(t, "Recipe", got.NewTypes[1].Name)
		assert.Equal(t, model.ObjectType_basic, got.NewTypes[1].Layout)
		assert.Equal(t, got.Containers["ds1"].TypeKey, got.Containers["ds2"].TypeKey)
		assert.NotEqual(t, got.Containers["ds1"].TypeKey, got.Containers["ds3"].TypeKey)
	})

	t.Run("static prompt is byte-identical across calls", func(t *testing.T) {
		// given — the shared prefix is what hosted prompt caches and local KV
		// caches serve; any per-call variation would defeat them
		fake := newFakeLLM(t,
			fakeReply{content: `{"kind": "Chore"}`},
			fakeReply{content: `{"kind": "Chore"}`},
			fakeReply{content: `{"kind": "Recipe"}`},
		)
		planner := newTestPlanner(t, fake, WithPerContainerCalls())

		// when
		_, err := planner.Plan(context.Background(), perContainerSchemas)

		// then
		require.NoError(t, err)
		require.Len(t, fake.requests, 3)
		for _, request := range fake.requests {
			assert.Equal(t, perContainerSystemPrompt, systemMessage(request))
		}
	})

	t.Run("evidence is the single container document", func(t *testing.T) {
		// given
		fake := newFakeLLM(t,
			fakeReply{content: `{"kind": "Chore"}`},
			fakeReply{content: `{"kind": "Chore"}`},
			fakeReply{content: `{"kind": "Recipe"}`},
		)
		planner := newTestPlanner(t, fake, WithPerContainerCalls())

		// when
		_, err := planner.Plan(context.Background(), perContainerSchemas)

		// then — every user turn carries exactly its own container, ordinal 1
		require.NoError(t, err)
		require.Len(t, fake.requests, 3)
		first := userMessage(fake.requests[0])
		assert.Contains(t, first, `"n":1`)
		assert.Contains(t, first, `"name":"Chores"`)
		assert.NotContains(t, first, "Recipes")
	})

	t.Run("empty kind answer leaves the container to its typesuggest verdict", func(t *testing.T) {
		// given
		fake := newFakeLLM(t,
			fakeReply{content: `{"kind": ""}`},
			fakeReply{content: `{"kind": "Chore"}`},
			fakeReply{content: `{"kind": "Recipe"}`},
		)
		planner := newTestPlanner(t, fake, WithPerContainerCalls())

		// when
		got, err := planner.Plan(context.Background(), perContainerSchemas)

		// then — ds1 ("Chores") falls back to the naive container-name verdict
		require.NoError(t, err)
		require.Contains(t, got.Containers, "ds1")
		assert.Equal(t, domain.TypeKey("task"), got.Containers["ds1"].TypeKey)
		assert.Equal(t, "container name", got.Containers["ds1"].Reason)
	})
}
