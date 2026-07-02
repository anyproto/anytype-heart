package notion

import (
	"context"
	"encoding/json"
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
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type recordingSink struct {
	objects []*importv2.Object
	issues  []importv2.Issue
}

func (s *recordingSink) Object(ctx context.Context, o *importv2.Object) error {
	s.objects = append(s.objects, o)
	return nil
}

func (s *recordingSink) Issue(i importv2.Issue) { s.issues = append(s.issues, i) }
func (s *recordingSink) Progress(delta int64)   {}

func (s *recordingSink) byKey(sourceKey string) *importv2.Object {
	for _, o := range s.objects {
		if o.SourceKey == sourceKey {
			return o
		}
	}
	return nil
}

func (s *recordingSink) relationByName(name string) *importv2.Object {
	for _, o := range s.objects {
		if o.SbType == coresb.SmartBlockTypeRelation &&
			o.Payload.Details.GetString(bundle.RelationKeyName) == name {
			return o
		}
	}
	return nil
}

type stubFactory struct{}

func (stubFactory) MakeCollection(name string, memberSourceKeys []string) (*importv2.Object, error) {
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName: domain.String(name),
	})
	return &importv2.Object{
		SbType:  coresb.SmartBlockTypePage,
		Payload: &importv2.Snapshot{Details: details, ObjectTypes: []string{bundle.TypeKeyCollection.String()}},
	}, nil
}

// scriptedWorkspace is a hand-written API fake: one database with two
// property flavors (incl. the Tags redirect), one database page exercising
// blocks (nesting fixes, synced content, table headers, mentions), one
// workspace-level page.
func scriptedWorkspace(t *testing.T) http.HandlerFunc {
	t.Helper()
	routes := map[string]string{}

	searchPage1 := `{"results":[
		{"object":"database","id":"db1","parent":{"type":"workspace","workspace":true},
		 "title":[{"plain_text":"Tasks","type":"text"}]},
		{"object":"page","id":"p1","parent":{"type":"database_id","database_id":"db1"},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"Alpha","type":"text"}]}}}
	],"has_more":true,"next_cursor":"cursor-2"}`
	searchPage2 := `{"results":[
		{"object":"page","id":"p2","parent":{"type":"workspace","workspace":true},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"Beta","type":"text"}]}}},
		{"object":"page","id":"n1","parent":{"type":"page_id","page_id":"p1"},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"NoteChild","type":"text"}]}}},
		{"object":"page","id":"n2","parent":{"type":"block_id","block_id":"foreign-block"},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"NoteChild","type":"text"}]}}}
	],"has_more":false,"next_cursor":null}`

	routes["GET /databases/db1"] = `{
		"id":"db1","title":[{"plain_text":"Tasks","type":"text"}],
		"created_time":"2024-01-01T10:00:00.000Z","last_edited_time":"2024-01-02T10:00:00.000Z",
		"properties":{
			"Name":{"id":"title","type":"title","name":"Name"},
			"Priority":{"id":"prio","type":"select","select":{"options":[
				{"id":"o1","name":"High","color":"red"},{"id":"o2","name":"Low","color":"brown"}]}},
			"Tags":{"id":"tags","type":"multi_select","multi_select":{"options":[
				{"id":"o3","name":"urgent","color":"gray"}]}},
			"Score":{"id":"score","type":"number"}
		}}`

	routes["GET /pages/p1"] = `{
		"id":"p1","archived":false,"icon":{"type":"emoji","emoji":"🔥"},
		"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
		"properties":{
			"Name":{"id":"title","type":"title","title":[{"plain_text":"Alpha","type":"text"}]},
			"Priority":{"id":"prio","type":"select","select":{"id":"o1","name":"High","color":"red"}},
			"Score":{"id":"score","type":"number","number":4.5},
			"Due":{"id":"due","type":"date","date":{"start":"2024-03-05","end":"2024-03-07"}}
		}}`
	routes["GET /pages/p2"] = `{
		"id":"p2","archived":false,
		"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
		"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Beta","type":"text"}]}}}`

	routes["GET /blocks/p1/children"] = `{"results":[
		{"id":"b1","type":"paragraph","has_children":false,"paragraph":{"rich_text":[
			{"plain_text":"see ","type":"text","annotations":{"bold":true}},
			{"plain_text":"Beta","type":"mention","mention":{"type":"page","page":{"id":"p2"}}}]}},
		{"id":"b2","type":"to_do","has_children":true,"to_do":{"rich_text":[{"plain_text":"task","type":"text"}],"checked":true}},
		{"id":"b3","type":"heading_1","has_children":true,"heading_1":{"rich_text":[{"plain_text":"Head","type":"text"}],"is_toggleable":true}},
		{"id":"b4","type":"synced_block","has_children":false,"synced_block":{"synced_from":{"block_id":"orig1"}}},
		{"id":"tab1-e5f6","type":"table","has_children":true,"table":{"table_width":2,"has_column_header":true,"has_row_header":false}},
		{"id":"cp1","type":"child_page","has_children":false,"child_page":{"title":"NoteChild"}}
	],"has_more":false,"next_cursor":null}`
	routes["GET /blocks/b2/children"] = `{"results":[
		{"id":"b2c","type":"paragraph","has_children":false,"paragraph":{"rich_text":[{"plain_text":"subtask","type":"text"}]}}
	],"has_more":false,"next_cursor":null}`
	routes["GET /blocks/b3/children"] = `{"results":[
		{"id":"b3c","type":"paragraph","has_children":false,"paragraph":{"rich_text":[{"plain_text":"under heading","type":"text"}]}}
	],"has_more":false,"next_cursor":null}`
	routes["GET /blocks/orig1/children"] = `{"results":[
		{"id":"sc1","type":"paragraph","has_children":false,"paragraph":{"rich_text":[{"plain_text":"synced content","type":"text"}]}}
	],"has_more":false,"next_cursor":null}`
	routes["GET /blocks/tab1-e5f6/children"] = `{"results":[
		{"id":"row1-a1b2","type":"table_row","has_children":false,"table_row":{"cells":[[{"plain_text":"H1","type":"text"}],[{"plain_text":"H2","type":"text"}]]}},
		{"id":"row2-c3d4","type":"table_row","has_children":false,"table_row":{"cells":[[{"plain_text":"a","type":"text"}],[{"plain_text":"b","type":"text"}]]}}
	],"has_more":false,"next_cursor":null}`
	routes["GET /blocks/p2/children"] = `{"results":[],"has_more":false,"next_cursor":null}`

	emptyPage := func(id, title string) string {
		return `{"id":"` + id + `","archived":false,
			"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
			"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"` + title + `","type":"text"}]}}}`
	}
	routes["GET /pages/n1"] = emptyPage("n1", "NoteChild")
	routes["GET /pages/n2"] = emptyPage("n2", "NoteChild")
	routes["GET /blocks/n1/children"] = `{"results":[],"has_more":false,"next_cursor":null}`
	routes["GET /blocks/n2/children"] = `{"results":[],"has_more":false,"next_cursor":null}`

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/search" {
			var body struct {
				StartCursor string `json:"start_cursor"`
			}
			_ = jsonDecode(r, &body)
			if body.StartCursor == "" {
				fmt.Fprint(w, searchPage1)
			} else {
				fmt.Fprint(w, searchPage2)
			}
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

func jsonDecode(r *http.Request, out any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(out)
}

func runScripted(t *testing.T) (*recordingSink, importv2.RootSpec, []importv2.IdentityClaim) {
	t.Helper()
	server := httptest.NewServer(scriptedWorkspace(t))
	t.Cleanup(server.Close)
	apiClient := client.NewClient("token",
		client.WithBaseURL(server.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())

	var claims []importv2.IdentityClaim
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(c importv2.IdentityClaim) error {
		claims = append(claims, c)
		return nil
	}))
	sink := &recordingSink{}
	rootSpec, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)
	return sink, rootSpec, claims
}

func TestScriptedWorkspace(t *testing.T) {
	sink, rootSpec, claims := runScripted(t)

	t.Run("pass 1 claims every entity once", func(t *testing.T) {
		require.Len(t, claims, 5)
		assert.Equal(t, "Notion Import", rootSpec.CollectionName)
		assert.Equal(t, model.BlockContentWidget_CompactList, rootSpec.WidgetLayout)
	})

	t.Run("database becomes a collection with its pages and seeded schema", func(t *testing.T) {
		collection := sink.byKey("db1")
		require.NotNil(t, collection)
		assert.True(t, collection.IsRootCandidate)
		assert.Equal(t, "Tasks", collection.Payload.Details.GetString(bundle.RelationKeyName))

		priority := sink.relationByName("Priority")
		require.NotNil(t, priority, "select property emits a relation")
		assert.Equal(t, int64(model.RelationFormat_tag), priority.Payload.Details.GetInt64(bundle.RelationKeyRelationFormat))

		score := sink.relationByName("Score")
		require.NotNil(t, score)
		assert.Equal(t, int64(model.RelationFormat_number), score.Payload.Details.GetInt64(bundle.RelationKeyRelationFormat))

		assert.Nil(t, sink.relationByName("Tags"),
			"Tags multi_select redirects to the bundled tag relation, no new relation")
		urgent := sink.byKey("option:tag:urgent")
		require.NotNil(t, urgent, "redirected option lands on the bundled tag key")
		assert.Equal(t, "grey", urgent.Payload.Details.GetString(bundle.RelationKeyRelationOptionColor))

		priorityKey := priority.Payload.Key
		low := sink.byKey("option:" + priorityKey + ":Low")
		require.NotNil(t, low)
		assert.Equal(t, "orange", low.Payload.Details.GetString(bundle.RelationKeyRelationOptionColor),
			"brown maps to its nearest anytype hue")
	})

	t.Run("page details: options, numbers, date range companion", func(t *testing.T) {
		page := sink.byKey("p1")
		require.NotNil(t, page)
		assert.False(t, page.IsRootCandidate, "parented to an imported database")
		assert.Equal(t, "Alpha", page.Payload.Details.GetString(bundle.RelationKeyName))
		assert.Equal(t, "🔥", page.Payload.Details.GetString(bundle.RelationKeyIconEmoji))

		priorityKey := sink.relationByName("Priority").Payload.Key
		assert.Equal(t, []string{"option:" + priorityKey + ":High"},
			page.Payload.Details.GetStringList(domain.RelationKey(priorityKey)))

		scoreKey := sink.relationByName("Score").Payload.Key
		assert.Equal(t, 4.5, page.Payload.Details.GetFloat64(domain.RelationKey(scoreKey)))

		due := sink.relationByName("Due")
		require.NotNil(t, due)
		dueEnd := sink.relationByName("Due (end)")
		require.NotNil(t, dueEnd, "date range end becomes a companion relation")
		assert.Positive(t, page.Payload.Details.GetInt64(domain.RelationKey(due.Payload.Key)))
		assert.Positive(t, page.Payload.Details.GetInt64(domain.RelationKey(dueEnd.Payload.Key)))
	})

	t.Run("blocks: nesting, synced content, mentions, table headers", func(t *testing.T) {
		page := sink.byKey("p1")
		require.NotNil(t, page)
		blocks := map[string]*model.Block{}
		for _, b := range page.Payload.Blocks {
			blocks[b.Id] = b
		}

		mentionBlock := blocks["b1"]
		require.NotNil(t, mentionBlock)
		marks := mentionBlock.GetText().GetMarks().GetMarks()
		require.NotEmpty(t, marks)
		var mentionParam string
		for _, mark := range marks {
			if mark.Type == model.BlockContentTextMark_Mention {
				mentionParam = mark.Param
			}
		}
		assert.Equal(t, "p2", mentionParam)

		todo := blocks["b2"]
		require.NotNil(t, todo)
		assert.True(t, todo.GetText().Checked)
		assert.Equal(t, []string{"b2c"}, todo.ChildrenIds, "to_do children nest exactly once")

		heading := blocks["b3"]
		require.NotNil(t, heading)
		assert.Equal(t, []string{"b3c"}, heading.ChildrenIds, "toggleable heading keeps its children (v1 flattened)")

		require.NotNil(t, blocks["sc1"], "synced-block content imported (v1 lost it)")

		row1 := blocks["rrow1a1b2"]
		require.NotNil(t, row1, "row ids are dash-free derivatives of the notion id")
		assert.True(t, row1.GetTableRow().IsHeader, "has_column_header marks the first row (v1 inverted)")
		row2 := blocks["rrow2c3d4"]
		require.NotNil(t, row2)
		assert.False(t, row2.GetTableRow().IsHeader)
	})

	t.Run("table cell ids satisfy the rowID-colID single-dash invariant", func(t *testing.T) {
		// ParseCellID splits on the FIRST dash and IsTableCell rejects
		// multi-dash ids; dashed notion UUIDs corrupted every table before.
		page := sink.byKey("p1")
		require.NotNil(t, page)
		cells := 0
		for _, b := range page.Payload.Blocks {
			if row := b.GetTableRow(); row != nil {
				assert.NotContains(t, b.Id, "-", "row id must be dash-free")
				for _, cellId := range b.ChildrenIds {
					assert.Equal(t, 1, strings.Count(cellId, "-"),
						"cell id %q must contain exactly one dash", cellId)
					assert.True(t, strings.HasPrefix(cellId, b.Id+"-"),
						"cell id %q must start with its row id", cellId)
					cells++
				}
			}
			if column := b.GetTableColumn(); column != nil {
				assert.NotContains(t, b.Id, "-", "column id must be dash-free")
			}
		}
		assert.Equal(t, 4, cells)
	})

	t.Run("child_page resolves to this page's subpage, ignoring foreign block-parented twins", func(t *testing.T) {
		// n1 (parent page_id=p1) and n2 (parent block_id of ANOTHER page)
		// share the title; treating every block-parented entity as local
		// used to make this ambiguous and degrade the link.
		page := sink.byKey("p1")
		require.NotNil(t, page)
		var childLink string
		for _, b := range page.Payload.Blocks {
			if b.Id == "cp1" {
				require.NotNil(t, b.GetLink(), "child_page must resolve to a link, not a placeholder")
				childLink = b.GetLink().TargetBlockId
			}
		}
		assert.Equal(t, "n1", childLink)
	})

	t.Run("workspace-level page is a root candidate", func(t *testing.T) {
		page := sink.byKey("p2")
		require.NotNil(t, page)
		assert.True(t, page.IsRootCandidate)
	})
}
