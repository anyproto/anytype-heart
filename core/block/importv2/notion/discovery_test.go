package notion

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// discoveryWorkspace scripts the GO-5273 shape: /search returns only p1, but
// p1's block tree references children /search omitted — a child page chain
// (ghost1 → ghost2), a child database (ghostdb → data source ghostds), a
// link_to_page target (ghost3), and one genuinely inaccessible page (denied).
func discoveryWorkspace(t *testing.T) http.HandlerFunc {
	t.Helper()
	emptyPage := func(id, title, parentType, parentId string) string {
		parent := `{"type":"workspace","workspace":true}`
		if parentType == "page_id" {
			parent = `{"type":"page_id","page_id":"` + parentId + `"}`
		}
		return `{"id":"` + id + `","archived":false,"parent":` + parent + `,
			"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
			"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"` + title + `","type":"text"}]}}}`
	}
	noChildren := `{"results":[],"has_more":false,"next_cursor":null}`
	routes := map[string]string{
		"GET /pages/p1":     emptyPage("p1", "Root", "workspace", ""),
		"GET /pages/ghost1": emptyPage("ghost1", "GhostChild", "page_id", "p1"),
		"GET /pages/ghost2": emptyPage("ghost2", "GhostGrandchild", "page_id", "ghost1"),
		"GET /pages/ghost3": emptyPage("ghost3", "GhostLinked", "page_id", "p1"),
		"GET /blocks/p1/children": `{"results":[
			{"id":"ghost1","type":"child_page","has_children":true,"child_page":{"title":"GhostChild"}},
			{"id":"denied","type":"child_page","has_children":true,"child_page":{"title":"Secret"}},
			{"id":"ghostdb","type":"child_database","has_children":false,"child_database":{"title":"GhostBase"}},
			{"id":"l1","type":"link_to_page","has_children":false,"link_to_page":{"type":"page_id","page_id":"ghost3"}}
		],"has_more":false,"next_cursor":null}`,
		"GET /blocks/ghost1/children": `{"results":[
			{"id":"ghost2","type":"child_page","has_children":true,"child_page":{"title":"GhostGrandchild"}}
		],"has_more":false,"next_cursor":null}`,
		"GET /blocks/ghost2/children": noChildren,
		"GET /blocks/ghost3/children": noChildren,
		"GET /databases/ghostdb": `{"id":"ghostdb","title":[{"plain_text":"GhostBase","type":"text"}],
			"parent":{"type":"page_id","page_id":"p1"},
			"data_sources":[{"id":"ghostds","name":"GhostBase"}]}`,
		"GET /data_sources/ghostds": `{"id":"ghostds","title":[{"plain_text":"GhostBase","type":"text"}],
			"created_time":"2024-01-01T10:00:00.000Z","last_edited_time":"2024-01-02T10:00:00.000Z",
			"properties":{"Name":{"id":"title","type":"title","name":"Name"}}}`,
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/search" {
			fmt.Fprint(w, `{"results":[
				{"object":"page","id":"p1","parent":{"type":"workspace","workspace":true},
				 "properties":{"Name":{"type":"title","title":[{"plain_text":"Root","type":"text"}]}}}
			],"has_more":false,"next_cursor":null}`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/pages/denied" {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"object":"error","status":404,"code":"object_not_found","message":"Could not find page"}`)
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

func TestSecondChanceDiscovery(t *testing.T) {
	// given
	server := httptest.NewServer(discoveryWorkspace(t))
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
	require.Len(t, claims, 1, "search sees only p1")

	// when
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	t.Run("omitted children are claimed late and imported", func(t *testing.T) {
		lateKeys := make([]string, 0, len(sink.claims))
		for _, claim := range sink.claims {
			lateKeys = append(lateKeys, claim.SourceKey)
		}
		assert.ElementsMatch(t, []string{"ghost1", "ghost2", "ghost3", "ghostds"}, lateKeys)
		for _, key := range []string{"ghost1", "ghost2", "ghost3", "ghostds"} {
			assert.NotNil(t, sink.byKey(key), "discovered entity %s must be emitted", key)
		}
	})

	t.Run("child blocks resolve to the discovered targets", func(t *testing.T) {
		page := sink.byKey("p1")
		require.NotNil(t, page)
		targets := map[string]string{}
		for _, b := range page.Payload.Blocks {
			if link := b.GetLink(); link != nil {
				targets[b.Id] = link.TargetBlockId
			}
		}
		assert.Equal(t, "ghost1", targets["ghost1"], "child_page resolves via discovery")
		assert.Equal(t, "ghostds", targets["ghostdb"], "child_database resolves via its data source")
		assert.Equal(t, "ghost3", targets["l1"], "link_to_page resolves via discovery")
	})

	t.Run("chained discovery reaches grandchildren", func(t *testing.T) {
		ghost1 := sink.byKey("ghost1")
		require.NotNil(t, ghost1)
		var target string
		for _, b := range ghost1.Payload.Blocks {
			if link := b.GetLink(); link != nil && b.Id == "ghost2" {
				target = link.TargetBlockId
			}
		}
		assert.Equal(t, "ghost2", target)
	})

	t.Run("inaccessible page keeps the missing-target degrade", func(t *testing.T) {
		page := sink.byKey("p1")
		require.NotNil(t, page)
		var placeholder string
		for _, b := range page.Payload.Blocks {
			if b.Id == "denied" {
				placeholder = b.GetText().GetText()
			}
		}
		assert.Equal(t, "Unresolved link: Secret", placeholder)
		var warned bool
		for _, issue := range sink.issues {
			if issue.Code == importv2.IssueMissingTarget && issue.SourceKey == "p1" {
				warned = true
			}
		}
		assert.True(t, warned, "the 404 child must still be reported")
	})

	t.Run("discovered database imports as a collection", func(t *testing.T) {
		collection := sink.byKey("ghostds")
		require.NotNil(t, collection)
		assert.Contains(t, collection.Payload.ObjectTypes, bundle.TypeKeyCollection.String())
	})
}
