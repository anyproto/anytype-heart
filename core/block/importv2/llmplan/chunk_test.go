package llmplan

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func chunkSchemas(n int) []schemaplan.ContainerSchema {
	out := make([]schemaplan.ContainerSchema, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, schemaplan.ContainerSchema{
			Id:   fmt.Sprintf("c%02d", i),
			Name: fmt.Sprintf("DB %02d", i),
			Properties: []schemaplan.PropertySchema{
				{Id: "p1", Name: "Title", Format: model.RelationFormat_longtext},
			},
		})
	}
	return out
}

// one kind per chunk, always named "Task", claiming every ordinal in the chunk
func taskReply(size int) string {
	ordinals := make([]string, 0, size)
	for i := 1; i <= size; i++ {
		ordinals = append(ordinals, fmt.Sprint(i))
	}
	return `{"kinds":[{"name_singular":"Task","name_plural":"Tasks","icon":"checkbox",` +
		`"layout":"todo","containers":[` + strings.Join(ordinals, ",") + `],"featured":[]}]}`
}

func TestPlanChunked(t *testing.T) {
	t.Run("ordinals resolve per chunk, not globally", func(t *testing.T) {
		// given — 5 containers in chunks of 2 means chunk 3 has one container
		// whose ordinal is 1, and it must resolve to c04, not c00.
		schemas := chunkSchemas(5)
		fake := newFakeLLM(t, content(taskReply(2), taskReply(2), taskReply(1))...)
		planner := newTestPlanner(t, fake, WithChunkSize(2))

		// when
		plan, err := planner.Plan(context.Background(), schemas)

		// then
		require.NoError(t, err)
		for _, schema := range schemas {
			require.Contains(t, plan.Containers, schema.Id, "container %s unassigned", schema.Id)
		}
	})

	t.Run("same kind name across chunks merges into one type", func(t *testing.T) {
		// given
		schemas := chunkSchemas(4)
		fake := newFakeLLM(t, content(taskReply(2), taskReply(2))...)
		planner := newTestPlanner(t, fake, WithChunkSize(2))

		// when
		plan, err := planner.Plan(context.Background(), schemas)

		// then — one kind, not one per chunk
		require.NoError(t, err)
		require.Len(t, plan.NewTypes, 1)
		assert.Equal(t, "Task", plan.NewTypes[0].Name)
		first := plan.Containers["c00"].TypeKey
		assert.Equal(t, first, plan.Containers["c03"].TypeKey,
			"containers from different chunks must share the merged type")
	})

	t.Run("a failing chunk degrades only its own containers", func(t *testing.T) {
		// given — chunk 1 answers, chunk 2 is unparseable twice (call + retry)
		schemas := chunkSchemas(4)
		fake := newFakeLLM(t, content(taskReply(2), "not json", "still not json")...)
		planner := newTestPlanner(t, fake, WithChunkSize(2))

		// when
		plan, err := planner.Plan(context.Background(), schemas)

		// then
		require.NoError(t, err, "one bad chunk must not fail the whole plan")
		assert.Contains(t, plan.Containers, "c00")
		assert.Contains(t, plan.Containers, "c01")
	})

	t.Run("every chunk failing is a failed plan, not an empty one", func(t *testing.T) {
		// given
		schemas := chunkSchemas(4)
		fake := newFakeLLM(t, content("nope", "nope", "nope", "nope")...)
		planner := newTestPlanner(t, fake, WithChunkSize(2))

		// when
		_, err := planner.Plan(context.Background(), schemas)

		// then — silence here would hand back a naive plan reporting success
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chunks failed")
	})

	t.Run("chunking is off unless the workspace exceeds the chunk size", func(t *testing.T) {
		// given
		schemas := chunkSchemas(3)
		fake := newFakeLLM(t, content(taskReply(3))...)
		planner := newTestPlanner(t, fake, WithChunkSize(10))

		// when
		_, err := planner.Plan(context.Background(), schemas)

		// then — exactly one call, the unchunked path
		require.NoError(t, err)
		assert.Len(t, fake.requests, 1)
	})
}

func TestBalancedChunks(t *testing.T) {
	cases := []struct {
		n, max int
		want   []int // chunk sizes
	}{
		{35, 12, []int{12, 12, 11}},   // already healthy
		{35, 8, []int{7, 7, 7, 7, 7}}, // was 8,8,8,8,3 — the starved tail
		{37, 12, []int{10, 9, 9, 9}},  // was 12,12,12,1 — a 1-container tail
		{10, 12, []int{10}},           // under the threshold, one chunk
		{12, 12, []int{12}},           // exactly at it
		{13, 12, []int{7, 6}},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("n=%d max=%d", c.n, c.max), func(t *testing.T) {
			// when
			got := balancedChunks(c.n, c.max)

			// then
			var sizes []int
			covered := 0
			for i, bound := range got {
				sizes = append(sizes, bound[1]-bound[0])
				assert.Equal(t, covered, bound[0], "chunk %d must start where the last ended", i)
				covered = bound[1]
			}
			assert.Equal(t, c.want, sizes)
			assert.Equal(t, c.n, covered, "chunks must cover every container exactly once")
			for _, size := range sizes {
				assert.LessOrEqual(t, size, c.max, "no chunk may exceed the max")
			}
		})
	}
}
