package notion

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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

// issueMessages renders an issue the way the report does: the sentence, and
// the subject it is about.
func issueMessages(sink *recordingSink, code importv2.IssueCode) []string {
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
		// db1 is this type's only database, so the type took its source key
		// and no separate collection was emitted.
		typeObject := sink.byKey("db1")
		require.NotNil(t, typeObject, "plan type object emitted")
		assert.Equal(t, "Sprint", typeObject.Payload.Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, int64(model.ObjectType_todo), typeObject.Payload.Details.GetInt64(bundle.RelationKeyRecommendedLayout))
		assert.Equal(t, []string{bundle.RelationKeyDueDate.BundledURL()},
			typeObject.Payload.Details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))

		// the type definition's custom property exists exactly once, under the
		// shared plan key, and the container's Score resolves onto it
		effortKey := schemaplan.CustomRelationKey(schemaplan.ScopedKey("effort", "sprint")).String()
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
		assert.Contains(t, suggested[0], `Tasks → `+mintedType.String()+` (LLM plan)`)
		mapped := issueMessages(sink, importv2.IssuePropertyMapped)
		require.Len(t, mapped, 1)
		assert.Contains(t, mapped[0], "Imported under a different name")
		assert.Contains(t, mapped[0], "Score → Effort")
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
		effortKey := schemaplan.CustomRelationKey(schemaplan.ScopedKey("effort", "sprint")).String()
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
		assert.True(t, strings.Contains(suggested[0], `Tasks → task (container name)`), suggested[0])
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
		assert.Contains(t, suggested[0], `Tasks → task (container name)`)
		assert.Empty(t, issueMessages(sink, importv2.IssuePropertyMapped))
	})
}

// twoCategoryWorkspace is two databases whose select properties share a name —
// the shape that merged four real databases' "Category" option pools into one
// dropdown in the 2026-08-04 live import.
func twoCategoryWorkspace(t *testing.T) http.HandlerFunc {
	t.Helper()
	routes := map[string]string{
		"GET /data_sources/recipes": `{
			"id":"recipes","title":[{"plain_text":"Recipe SB","type":"text"}],
			"properties":{
				"Name":{"id":"title","type":"title","name":"Name"},
				"Category":{"id":"catA","type":"select","select":{"options":[
					{"id":"o1","name":"Breakfast","color":"red"},{"id":"o2","name":"Dinner","color":"blue"}]}}
			}}`,
		"GET /data_sources/launch": `{
			"id":"launch","title":[{"plain_text":"Launch Tracker","type":"text"}],
			"properties":{
				"Name":{"id":"title","type":"title","name":"Name"},
				"Category":{"id":"catB","type":"select","select":{"options":[
					{"id":"o3","name":"Marketing","color":"green"},{"id":"o4","name":"Sales","color":"gray"}]}}
			}}`,
		"GET /pages/pr1": `{"id":"pr1","archived":false,
			"properties":{
				"Name":{"id":"title","type":"title","title":[{"plain_text":"Toast","type":"text"}]},
				"Category":{"id":"catA","type":"select","select":{"id":"o1","name":"Breakfast","color":"red"}}}}`,
		"GET /pages/pl1": `{"id":"pl1","archived":false,
			"properties":{
				"Name":{"id":"title","type":"title","title":[{"plain_text":"Launch Twitter","type":"text"}]},
				"Category":{"id":"catB","type":"select","select":{"id":"o3","name":"Marketing","color":"green"}}}}`,
		"GET /blocks/pr1/children": `{"results":[],"has_more":false,"next_cursor":null}`,
		"GET /blocks/pl1/children": `{"results":[],"has_more":false,"next_cursor":null}`,
	}
	search := `{"results":[
		{"object":"data_source","id":"recipes","parent":{"type":"database_id","database_id":"realrecipes"},
		 "database_parent":{"type":"workspace","workspace":true},
		 "title":[{"plain_text":"Recipe SB","type":"text"}]},
		{"object":"data_source","id":"launch","parent":{"type":"database_id","database_id":"reallaunch"},
		 "database_parent":{"type":"workspace","workspace":true},
		 "title":[{"plain_text":"Launch Tracker","type":"text"}]},
		{"object":"page","id":"pr1","parent":{"type":"data_source_id","data_source_id":"recipes"},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"Toast","type":"text"}]}}},
		{"object":"page","id":"pl1","parent":{"type":"data_source_id","data_source_id":"launch"},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"Launch Twitter","type":"text"}]}}}
	],"has_more":false,"next_cursor":null}`

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/search" {
			fmt.Fprint(w, search)
			return
		}
		if response, ok := routes[r.Method+" "+r.URL.Path]; ok {
			fmt.Fprint(w, response)
			return
		}
		t.Errorf("unexpected api call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestPlanNeverMergesSelectVocabularies(t *testing.T) {
	// given — the model maps both databases' Category onto one shared key,
	// which is exactly what the prompt used to ask for
	planner := schemaplan.PlannerFunc(func(_ context.Context, schemas []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
		containers := map[string]schemaplan.ContainerPlan{}
		for _, schema := range schemas {
			containers[schema.Id] = schemaplan.ContainerPlan{
				Reason:     "LLM plan",
				Properties: map[string]schemaplan.PropertyPlan{schema.Properties[0].Id: {Key: "category", Name: "Category"}},
			}
		}
		return schemaplan.Plan{Containers: containers}, nil
	})

	server := httptest.NewServer(twoCategoryWorkspace(t))
	t.Cleanup(server.Close)
	apiClient := client.NewClient("token",
		client.WithBaseURL(server.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir(), WithPlanner(planner))
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))

	// when
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	// then — two relations, and neither offers the other's vocabulary
	recipeKey := schemaplan.CustomRelationKey(schemaplan.ScopedKey("category", "recipes")).String()
	launchKey := schemaplan.CustomRelationKey(schemaplan.ScopedKey("category", "launch")).String()
	require.NotEqual(t, recipeKey, launchKey)
	require.NotNil(t, sink.byKey("relation:"+recipeKey), "recipe Category relation missing")
	require.NotNil(t, sink.byKey("relation:"+launchKey), "launch Category relation missing")

	assert.NotNil(t, sink.byKey("option:"+recipeKey+":Breakfast"))
	assert.NotNil(t, sink.byKey("option:"+launchKey+":Marketing"))
	assert.Nil(t, sink.byKey("option:"+recipeKey+":Marketing"),
		"a recipe's Category must not offer the launch vocabulary")
	assert.Nil(t, sink.byKey("option:"+launchKey+":Breakfast"),
		"a launch item's Category must not offer the meal vocabulary")
}

func TestNaiveNeverMergesPropertiesAcrossDatabases(t *testing.T) {
	// given — two databases with same-named select properties and no plan at
	// all. In the 2026-08-04 live workspace "Status" was shared by 18
	// databases, so every board grouped by Status carried seventeen other
	// databases' empty columns.
	server := httptest.NewServer(twoCategoryWorkspace(t))
	t.Cleanup(server.Close)
	apiClient := client.NewClient("token",
		client.WithBaseURL(server.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))

	// when
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	// then — a property belongs to its own database, so each Category keeps
	// its own vocabulary
	var optionOwners []string
	for _, object := range sink.objects {
		if strings.HasPrefix(object.SourceKey, "option:") {
			optionOwners = append(optionOwners, object.SourceKey)
		}
	}
	require.Len(t, optionOwners, 4, "each database contributes its own two options")

	relations := map[string]bool{}
	for _, key := range optionOwners {
		relations[strings.Split(key, ":")[1]] = true
	}
	assert.Len(t, relations, 2, "the two databases' Category vocabularies merged into one option pool")
}

func TestSharedTypeBecomesCollectionsPlusOneType(t *testing.T) {
	// given — two databases the plan calls the same kind of thing
	planner := schemaplan.PlannerFunc(func(_ context.Context, schemas []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
		containers := map[string]schemaplan.ContainerPlan{}
		for _, schema := range schemas {
			containers[schema.Id] = schemaplan.ContainerPlan{
				TypeKey:    "catalogued",
				Reason:     "LLM plan",
				Properties: map[string]schemaplan.PropertyPlan{schema.Properties[0].Id: {Key: "category", Name: "Category"}},
			}
		}
		return schemaplan.Plan{
			NewTypes: []schemaplan.TypeDefinition{{
				Key: "catalogued", Name: "Catalogued item",
				Properties: []schemaplan.TypeProperty{{Key: "category", Name: "Category", Format: model.RelationFormat_status}},
			}},
			Containers: containers,
		}, nil
	})

	server := httptest.NewServer(twoCategoryWorkspace(t))
	t.Cleanup(server.Close)
	apiClient := client.NewClient("token",
		client.WithBaseURL(server.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir(), WithPlanner(planner))
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))

	// when
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	// then — two collections, one type: the databases keep their identity as
	// lists while their rows share a shape
	collections, types := 0, 0
	for _, object := range sink.objects {
		if object.Payload == nil || len(object.Payload.ObjectTypes) == 0 {
			continue
		}
		switch object.Payload.ObjectTypes[0] {
		case bundle.TypeKeyCollection.String():
			collections++
		case bundle.TypeKeyObjectType.String():
			types++
		}
	}
	assert.Equal(t, 2, collections, "each database stays its own collection")
	assert.Equal(t, 1, types, "one shape for one kind of thing")

	minted := schemaplan.CustomTypeKey("catalogued").String()
	for _, key := range []string{"pr1", "pl1"} {
		page := sink.byKey(key)
		require.NotNil(t, page, key)
		assert.Equal(t, []string{minted}, page.Payload.ObjectTypes)
	}

	// one relation for the shared type, holding both databases' vocabulary —
	// correct here precisely because they ARE one type
	sharedKey := schemaplan.CustomRelationKey(schemaplan.ScopedKey("category", "catalogued")).String()
	require.NotNil(t, sink.byKey("relation:"+sharedKey))
	for _, option := range []string{"Breakfast", "Dinner", "Marketing", "Sales"} {
		assert.NotNil(t, sink.byKey("option:"+sharedKey+":"+option), option)
	}
}

func TestSingleDatabaseTypeReplacesItsCollection(t *testing.T) {
	// given — one database, one minted type: "all objects of this type" and
	// "members of this collection" would be the same list
	planner := schemaplan.PlannerFunc(func(_ context.Context, schemas []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
		return schemaplan.Plan{
			NewTypes: []schemaplan.TypeDefinition{{Key: "sprint", Name: "Sprint", Layout: model.ObjectType_todo}},
			Containers: map[string]schemaplan.ContainerPlan{
				"db1": {TypeKey: "sprint", Reason: "LLM plan"},
			},
		}, nil
	})

	// when
	sink := runScriptedWithOptions(t, WithPlanner(planner))

	// then — the database's object IS the type, so links to it still resolve
	object := sink.byKey("db1")
	require.NotNil(t, object, "the database's source key must still be emitted")
	assert.Equal(t, coresb.SmartBlockTypeObjectType, object.SbType,
		"a single-database type replaces its collection rather than duplicating it")
	assert.Equal(t, "Sprint", object.Payload.Details.GetString(bundle.RelationKeyName))
	assert.True(t, object.IsRootCandidate, "the import root must still surface it")

	// no collection was emitted for that database
	for _, emitted := range sink.objects {
		if emitted.SourceKey == "db1" {
			continue
		}
		if emitted.Payload != nil && len(emitted.Payload.ObjectTypes) > 0 {
			assert.NotEqual(t, bundle.TypeKeyCollection.String(), emitted.Payload.ObjectTypes[0],
				"no separate collection for a single-database type")
		}
	}

	// rows still carry the minted type
	page := sink.byKey("p1")
	require.NotNil(t, page)
	assert.Equal(t, []string{schemaplan.CustomTypeKey("sprint").String()}, page.Payload.ObjectTypes)
}

// overlappingContainersWorkspace returns a data_source stub and a bare
// database stub that resolves to the same data source, so both containers
// claim the same pages.
func overlappingContainersWorkspace(t *testing.T) http.HandlerFunc {
	t.Helper()
	schema := `{
		"id":"ds1","title":[{"plain_text":"Tasks","type":"text"}],
		"properties":{
			"Name":{"id":"title","type":"title","name":"Name"},
			"State":{"id":"st","type":"select","select":{"options":[{"id":"o1","name":"Open","color":"red"}]}}
		}}`
	routes := map[string]string{
		"GET /data_sources/ds1": schema,
		"GET /databases/realdb": `{"id":"realdb","data_sources":[{"id":"ds1"}]}`,
		"GET /pages/pg1": `{"id":"pg1","archived":false,
			"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"One","type":"text"}]}}}`,
		"GET /blocks/pg1/children": `{"results":[],"has_more":false,"next_cursor":null}`,
	}
	search := `{"results":[
		{"object":"data_source","id":"ds1","parent":{"type":"database_id","database_id":"realdb"},
		 "database_parent":{"type":"workspace","workspace":true},
		 "title":[{"plain_text":"Tasks","type":"text"}]},
		{"object":"database","id":"realdb","parent":{"type":"workspace","workspace":true},
		 "title":[{"plain_text":"Tasks DB","type":"text"}]},
		{"object":"page","id":"pg1","parent":{"type":"data_source_id","data_source_id":"ds1"},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"One","type":"text"}]}}}
	],"has_more":false,"next_cursor":null}`

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/search" {
			fmt.Fprint(w, search)
			return
		}
		if response, ok := routes[r.Method+" "+r.URL.Path]; ok {
			fmt.Fprint(w, response)
			return
		}
		t.Errorf("unexpected api call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestOverlappingContainersKeepTheirCollections(t *testing.T) {
	// given — two containers claiming the same page, each given its own type.
	// A page carries exactly one type, so at most one of these could ever be
	// represented by a type; the other's membership needs a collection.
	planner := schemaplan.PlannerFunc(func(_ context.Context, schemas []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
		plan := schemaplan.Plan{Containers: map[string]schemaplan.ContainerPlan{}}
		for i, schema := range schemas {
			key := domain.TypeKey(fmt.Sprintf("kind%d", i))
			plan.NewTypes = append(plan.NewTypes, schemaplan.TypeDefinition{Key: key, Name: fmt.Sprintf("Kind %d", i)})
			plan.Containers[schema.Id] = schemaplan.ContainerPlan{TypeKey: key, Reason: "LLM plan"}
		}
		return plan, nil
	})

	server := httptest.NewServer(overlappingContainersWorkspace(t))
	t.Cleanup(server.Close)
	apiClient := client.NewClient("token",
		client.WithBaseURL(server.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir(), WithPlanner(planner))
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))

	// when
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	// then — both containers still emit a collection, so no membership is lost
	for _, containerId := range []string{"ds1", "realdb"} {
		object := sink.byKey(containerId)
		require.NotNil(t, object, containerId)
		assert.Equal(t, coresb.SmartBlockTypePage, object.SbType,
			"containers sharing members must stay collections: a page cannot hold two types")
	}
}

func TestArchivedDatabaseDoesNotArchiveItsType(t *testing.T) {
	// given — an archived database that solely backs a minted type. A
	// collection in the bin is recoverable content; a TYPE in the bin is
	// referenced by every live row that carries it.
	planner := schemaplan.PlannerFunc(func(_ context.Context, schemas []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
		return schemaplan.Plan{
			NewTypes:   []schemaplan.TypeDefinition{{Key: "sprint", Name: "Sprint"}},
			Containers: map[string]schemaplan.ContainerPlan{"db1": {TypeKey: "sprint", Reason: "LLM plan"}},
		}, nil
	})

	server := httptest.NewServer(archivedDatabaseWorkspace(t))
	t.Cleanup(server.Close)
	apiClient := client.NewClient("token",
		client.WithBaseURL(server.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir(), WithPlanner(planner))
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))

	// when
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	// then
	object := sink.byKey("db1")
	require.NotNil(t, object)
	require.Equal(t, coresb.SmartBlockTypeObjectType, object.SbType)
	assert.False(t, object.Archived,
		"a type in the bin would strand every live object that carries it")
}

func archivedDatabaseWorkspace(t *testing.T) http.HandlerFunc {
	t.Helper()
	routes := map[string]string{
		"GET /data_sources/db1": `{"id":"db1","archived":true,
			"title":[{"plain_text":"Old Tasks","type":"text"}],
			"properties":{"Name":{"id":"title","type":"title","name":"Name"}}}`,
		"GET /pages/pg1": `{"id":"pg1","archived":false,
			"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Row","type":"text"}]}}}`,
		"GET /blocks/pg1/children": `{"results":[],"has_more":false,"next_cursor":null}`,
	}
	search := `{"results":[
		{"object":"data_source","id":"db1","parent":{"type":"database_id","database_id":"realdb"},
		 "database_parent":{"type":"workspace","workspace":true},
		 "title":[{"plain_text":"Old Tasks","type":"text"}]},
		{"object":"page","id":"pg1","parent":{"type":"data_source_id","data_source_id":"db1"},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"Row","type":"text"}]}}}
	],"has_more":false,"next_cursor":null}`
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/search" {
			fmt.Fprint(w, search)
			return
		}
		if response, ok := routes[r.Method+" "+r.URL.Path]; ok {
			fmt.Fprint(w, response)
			return
		}
		t.Errorf("unexpected api call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestTypeBackfillsTheDatabaseSchema(t *testing.T) {
	// given — db1 "Tasks" carries Priority, Tags, Score and Due, but the model
	// declares only Score on the type it mints. The collection used to be the
	// surface that listed every property regardless of what the plan named.
	planner := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
		return schemaplan.Plan{
			NewTypes: []schemaplan.TypeDefinition{{
				Key: "sprint", Name: "Sprint",
				Properties: []schemaplan.TypeProperty{
					{Key: "score", Name: "Score", Format: model.RelationFormat_number, Featured: true},
				},
			}},
			Containers: map[string]schemaplan.ContainerPlan{
				"db1": {TypeKey: "sprint", Reason: "LLM plan",
					Properties: map[string]schemaplan.PropertyPlan{
						"score": {Key: "score", Name: "Score", Format: model.RelationFormat_number},
					}},
			},
		}, nil
	})

	// when
	sink := runScriptedWithOptions(t, WithPlanner(planner))

	// then — the model still chooses what is featured
	typeObject := sink.byKey("db1")
	require.NotNil(t, typeObject)
	require.Equal(t, coresb.SmartBlockTypeObjectType, typeObject.SbType)
	scoreRef := "relation:" + schemaplan.CustomRelationKey(schemaplan.ScopedKey("score", "sprint")).String()
	assert.Equal(t, []string{scoreRef},
		typeObject.Payload.Details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))

	// ...but every other property of the database's schema is still listed on
	// the type, so nothing the rows carry goes unlisted just because the model
	// did not enumerate it. (Due lives only on the page in this fixture, not in
	// db1's schema, so it is not among them.)
	recommended := typeObject.Payload.Details.GetStringList(bundle.RelationKeyRecommendedRelations)
	priority := sink.relationByName("Priority")
	require.NotNil(t, priority)
	assert.Contains(t, recommended, priority.SourceKey,
		"Priority is imported and carried by rows but listed nowhere")
	assert.Contains(t, recommended, bundle.RelationKeyTag.BundledURL(),
		"the Tags property redirected onto the bundled tag relation and is still the database's")
	assert.NotContains(t, recommended, scoreRef, "a featured property is not also regular")
}
