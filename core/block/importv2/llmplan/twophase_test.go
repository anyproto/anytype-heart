package llmplan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/ai/llmclient"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// routingLLM answers by inspecting the request, because phase 2 runs
// concurrently and a reply QUEUE would hand replies to whichever goroutine
// arrived first. Concurrency-safe for the same reason.
type routingLLM struct {
	*httptest.Server
	mu       sync.Mutex
	requests []map[string]any
	route    func(system, user string) string
}

func newRoutingLLM(t *testing.T, route func(system, user string) string) *routingLLM {
	f := &routingLLM{route: route}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		messages := body["messages"].([]any)
		system := messages[0].(map[string]any)["content"].(string)
		user := messages[1].(map[string]any)["content"].(string)
		f.mu.Lock()
		f.requests = append(f.requests, body)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": f.route(system, user)}}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *routingLLM) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

var twoPhaseSchemas = []schemaplan.ContainerSchema{
	{Id: "tasksA", Name: "Tasks", Properties: []schemaplan.PropertySchema{
		{Id: "a1", Name: "Status", Format: model.RelationFormat_status},
	}},
	{Id: "tasksB", Name: "Sprint tasks", Properties: []schemaplan.PropertySchema{
		{Id: "b1", Name: "Status", Format: model.RelationFormat_status},
	}},
	{Id: "recipes", Name: "Recipes", Properties: []schemaplan.PropertySchema{
		{Id: "r1", Name: "Category", Format: model.RelationFormat_status},
	}},
}

func TestTwoPhasePlan(t *testing.T) {
	t.Run("phase 1 assigns kinds and phase 2 fills each type's properties", func(t *testing.T) {
		// given — two containers are one kind, the third is another
		route := func(system, user string) string {
			if strings.Contains(system, "kind") && !strings.Contains(system, "typeProperties") {
				return `{"kinds":[
					{"key":"task","name":"Task","pluralName":"Tasks","icon":"checkbox","layout":"todo","containerIds":["tasksA","tasksB"]},
					{"key":"recipe","name":"Recipe","pluralName":"Recipes","icon":"restaurant","layout":"basic","containerIds":["recipes"]}]}`
			}
			if strings.Contains(user, "tasksA") {
				return `{"typeProperties":[{"key":"taskStatus","name":"Status","format":"select","section":"featured"}],
					"containers":[
						{"id":"tasksA","properties":[{"id":"a1","key":"taskStatus","name":"Status","format":"select"}]},
						{"id":"tasksB","properties":[{"id":"b1","key":"taskStatus","name":"Status","format":"select"}]}]}`
			}
			return `{"typeProperties":[{"key":"recipeCategory","name":"Category","format":"select","section":"regular"}],
				"containers":[{"id":"recipes","properties":[{"id":"r1","key":"recipeCategory","name":"Category","format":"select"}]}]}`
		}
		fake := newRoutingLLM(t, route)
		client, err := llmclient.New(llmclient.Config{Endpoint: fake.URL + "/v1", Model: "test", Token: "t"},
			llmclient.WithRetryPolicy(llmclient.RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond}))
		require.NoError(t, err)

		// when
		plan, err := NewTwoPhase(client).Plan(context.Background(), twoPhaseSchemas)

		// then — one identify call plus one extract call per type
		require.NoError(t, err)
		assert.Equal(t, 3, fake.calls())

		// the same kind shares one type, and its containers share the property
		require.Len(t, plan.NewTypes, 2)
		assert.Equal(t, domain.TypeKey("task"), plan.Containers["tasksA"].TypeKey)
		assert.Equal(t, domain.TypeKey("task"), plan.Containers["tasksB"].TypeKey)
		assert.Equal(t, domain.TypeKey("recipe"), plan.Containers["recipes"].TypeKey)
		assert.Equal(t, domain.RelationKey("taskStatus"), plan.Containers["tasksA"].Properties["a1"].Key)
		assert.Equal(t, domain.RelationKey("taskStatus"), plan.Containers["tasksB"].Properties["b1"].Key)

		// the type carries what phase 1 named and what phase 2 found
		byKey := map[domain.TypeKey]schemaplan.TypeDefinition{}
		for _, def := range plan.NewTypes {
			byKey[def.Key] = def
		}
		task := byKey["task"]
		assert.Equal(t, "Task", task.Name)
		assert.Equal(t, "Tasks", task.PluralName)
		assert.Equal(t, "checkbox", task.IconName)
		assert.Equal(t, model.ObjectType_todo, task.Layout)
		require.Len(t, task.Properties, 1)
		assert.True(t, task.Properties[0].Featured)
	})

	t.Run("one type's extraction failing costs only that type's properties", func(t *testing.T) {
		// given — the recipe extract returns garbage; the task extract is fine
		route := func(system, user string) string {
			if strings.Contains(system, "kind") && !strings.Contains(system, "typeProperties") {
				return `{"kinds":[
					{"key":"task","name":"Task","pluralName":"Tasks","icon":"checkbox","layout":"todo","containerIds":["tasksA"]},
					{"key":"recipe","name":"Recipe","pluralName":"Recipes","icon":"restaurant","layout":"basic","containerIds":["recipes"]}]}`
			}
			if strings.Contains(user, "tasksA") {
				return `{"typeProperties":[{"key":"taskStatus","name":"Status","format":"select","section":"featured"}],
					"containers":[{"id":"tasksA","properties":[{"id":"a1","key":"taskStatus","name":"Status","format":"select"}]}]}`
			}
			return `not json at all`
		}
		fake := newRoutingLLM(t, route)
		client, err := llmclient.New(llmclient.Config{Endpoint: fake.URL + "/v1", Model: "test", Token: "t"},
			llmclient.WithRetryPolicy(llmclient.RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond}))
		require.NoError(t, err)

		// when
		plan, err := NewTwoPhase(client).Plan(context.Background(), twoPhaseSchemas)

		// then — the whole plan does not fail; both types survive, and the
		// recipe container keeps its type but loses only its remaps
		require.NoError(t, err)
		assert.Equal(t, domain.TypeKey("task"), plan.Containers["tasksA"].TypeKey)
		assert.Equal(t, domain.RelationKey("taskStatus"), plan.Containers["tasksA"].Properties["a1"].Key)
		assert.Equal(t, domain.TypeKey("recipe"), plan.Containers["recipes"].TypeKey)
		assert.Empty(t, plan.Containers["recipes"].Properties)
	})

	t.Run("phase 1 failing fails the plan, so the converter degrades to naive", func(t *testing.T) {
		// given
		fake := newRoutingLLM(t, func(system, user string) string { return `garbage` })
		client, err := llmclient.New(llmclient.Config{Endpoint: fake.URL + "/v1", Model: "test", Token: "t"},
			llmclient.WithRetryPolicy(llmclient.RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond}))
		require.NoError(t, err)

		// when
		_, err = NewTwoPhase(client).Plan(context.Background(), twoPhaseSchemas)

		// then
		require.Error(t, err)
	})
}
