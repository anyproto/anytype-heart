//go:build livesmoke

package main

// A live end-to-end check of the set_type op against a real heart. The unit
// tests mock the editor, so the one thing they cannot see is the layout
// conversion itself — the title, first-block and done-property moves the
// converter makes on a real page — and whether the adapter's plain Apply
// commits it. Only a live run answers that.
//
//	go test ./cmd/apiv2eval/ -tags livesmoke -run TestLiveSetType -v -timeout 10m

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/apiv2eval/heartboot"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// typedDocument is the envelope member the eval's document struct leaves out.
type typedDocument struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

func readTyped(t *testing.T, raw []byte) typedDocument {
	t.Helper()
	var doc typedDocument
	require.NoError(t, json.Unmarshal(raw, &doc))
	return doc
}

func TestLiveSetType(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()

	heart, err := heartboot.Start(ctx, heartboot.Options{AccountName: "livesmoke", AppName: "livesmoke"})
	require.NoError(t, err)
	defer heart.Stop()
	t.Logf("heart %s at %s", heart.AccountId, heart.APIURL)

	api := newAPIClient(heart.APIURL, heart.APIKey, nil)
	spaceId, err := api.createSpace(ctx, "livesmoke")
	require.NoError(t, err)
	deadline := time.Now().Add(spaceReadyTimeout)
	for {
		if _, err := api.call(ctx, http.MethodGet, "/v2/spaces/"+spaceId, nil, nil, nil); err == nil {
			break
		}
		require.False(t, time.Now().After(deadline), "space never became readable")
		time.Sleep(500 * time.Millisecond)
	}

	const title = "Quarterly planning"
	objectId, err := api.createObject(ctx, spaceId, "page", title, "First paragraph of the plan.\n\nSecond paragraph.")
	require.NoError(t, err)
	objectPath := "/v2/spaces/" + url.PathEscape(spaceId) + "/objects/" + url.PathEscape(objectId)

	t.Run("page to task adds the done property and keeps the body", func(t *testing.T) {
		result, err := api.patchOps(ctx, spaceId, objectId, []any{map[string]any{"op": "set_type", "type": "task"}})
		require.NoError(t, err)
		t.Logf("set_type task -> %+v", result.DiffStats)

		doc, raw, err := api.getDocument(ctx, spaceId, objectId)
		require.NoError(t, err)
		t.Logf("after task:\n%s", raw)
		assert.Equal(t, "task", readTyped(t, raw).Type)
		assert.Contains(t, doc.allText(), "First paragraph of the plan.")
		name, _ := doc.stringProperty("name")
		assert.Equal(t, title, name)
	})

	t.Run("task to note moves the name into the first block", func(t *testing.T) {
		_, err := api.patchOps(ctx, spaceId, objectId, []any{map[string]any{"op": "set_type", "type": "note"}})
		require.NoError(t, err)

		doc, raw, err := api.getDocument(ctx, spaceId, objectId)
		require.NoError(t, err)
		t.Logf("after note:\n%s", raw)
		assert.Equal(t, "note", readTyped(t, raw).Type)
		texts := doc.blockTexts()
		require.NotEmpty(t, texts)
		assert.Equal(t, title, texts[0], "the note layout has no title: the name becomes the first block")
	})

	t.Run("note back to page lifts the first block into the name", func(t *testing.T) {
		_, err := api.patchOps(ctx, spaceId, objectId, []any{map[string]any{"op": "set_type", "type": "page"}})
		require.NoError(t, err)

		doc, raw, err := api.getDocument(ctx, spaceId, objectId)
		require.NoError(t, err)
		t.Logf("after page:\n%s", raw)
		assert.Equal(t, "page", readTyped(t, raw).Type)
		name, _ := doc.stringProperty("name")
		assert.Equal(t, title, name)
		assert.Contains(t, doc.allText(), "First paragraph of the plan.")
	})

	t.Run("a bundled type the space never installed is installed and taken", func(t *testing.T) {
		// Human (profile layout) is not part of a fresh space; the change
		// has to install it before the editor reads it from the store
		_, err := api.patchOps(ctx, spaceId, objectId, []any{map[string]any{"op": "set_type", "type": "Human"}})
		require.NoError(t, err)

		_, raw, err := api.getDocument(ctx, spaceId, objectId)
		require.NoError(t, err)
		t.Logf("after human:\n%s", raw)
		assert.Equal(t, bundle.TypeApiSlug(string(bundle.TypeKeyProfile)), readTyped(t, raw).Type,
			"the bundled Human type carries the profile key")

		_, err = api.patchOps(ctx, spaceId, objectId, []any{map[string]any{"op": "set_type", "type": "page"}})
		require.NoError(t, err)
	})

	t.Run("a block edit before a note-to-page change keeps the block's children", func(t *testing.T) {
		nested, err := api.createObject(ctx, spaceId, "note", "", "First line")
		require.NoError(t, err)
		before, _, err := api.getDocument(ctx, spaceId, nested)
		require.NoError(t, err)
		require.NotEmpty(t, before.Blocks)
		first := before.Blocks[0].Id
		// nest a real child under the first block: markdown alone yields
		// siblings, and a sibling would survive even the bug this guards
		_, err = api.patchOps(ctx, spaceId, nested, []any{
			map[string]any{"op": "insert_blocks", "inside": first, "markdown": "child item"},
		})
		require.NoError(t, err)
		before, _, err = api.getDocument(ctx, spaceId, nested)
		require.NoError(t, err)
		require.Len(t, before.Blocks, 2)
		require.Equal(t, float64(1), before.Blocks[1].Indent, "the child must be nested, not a sibling")

		result, err := api.patchOps(ctx, spaceId, nested, []any{
			map[string]any{"op": "update_block", "id": first, "set": map[string]any{"text": "Edited first line"}},
			map[string]any{"op": "set_type", "type": "page"},
		})
		require.NoError(t, err)
		t.Logf("update_block + set_type page -> %+v type_changed=%+v", result.DiffStats, result.TypeChanged)

		after, raw, err := api.getDocument(ctx, spaceId, nested)
		require.NoError(t, err)
		t.Logf("after:\n%s", raw)
		name, _ := after.stringProperty("name")
		assert.Equal(t, "Edited first line", name)
		assert.Contains(t, after.allText(), "child item", "the nested block must survive the conversion")
	})

	t.Run("two conversions in one batch keep the name and the body", func(t *testing.T) {
		// page → note → page in ONE state: the title block is detached by
		// the first conversion and must be re-linked by the second
		result, err := api.patchOps(ctx, spaceId, objectId, []any{
			map[string]any{"op": "set_type", "type": "note"},
			map[string]any{"op": "set_type", "type": "page"},
		})
		require.NoError(t, err)
		t.Logf("note then page -> type_changed=%+v", result.TypeChanged)
		require.NotNil(t, result.TypeChanged)
		assert.Equal(t, "page", result.TypeChanged.To)

		doc, raw, err := api.getDocument(ctx, spaceId, objectId)
		require.NoError(t, err)
		t.Logf("after note→page:\n%s", raw)
		assert.Equal(t, "page", readTyped(t, raw).Type)
		name, _ := doc.stringProperty("name")
		assert.Equal(t, title, name)
		assert.Contains(t, doc.allText(), "First paragraph of the plan.")
		assert.NotContains(t, doc.allText(), title, "the name must not linger as a body block")
	})

	t.Run("a type change and a property write land in one edit", func(t *testing.T) {
		_, err := api.patchOps(ctx, spaceId, objectId, []any{
			map[string]any{"op": "set_type", "type": "task"},
			map[string]any{"op": "set_properties", "set": map[string]any{"done": true}},
		})
		require.NoError(t, err)

		_, raw, err := api.getDocument(ctx, spaceId, objectId)
		require.NoError(t, err)
		doc := readTyped(t, raw)
		assert.Equal(t, "task", doc.Type)
		assert.Equal(t, true, doc.Properties["done"])
	})

	t.Run("page to collection is refused with the types the object can take", func(t *testing.T) {
		_, err := api.patchOps(ctx, spaceId, objectId, []any{map[string]any{"op": "set_type", "type": "collection"}})
		var apiErr *apiError
		require.ErrorAs(t, err, &apiErr)
		t.Logf("set_type collection -> %d %s\n%+v", apiErr.Status, apiErr.Message, apiErr.Issues)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Contains(t, apiErr.Message, "collection")
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].type", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "page")
		assert.NotEmpty(t, apiErr.Issues[0].SeeAlso)
	})

	t.Run("set_properties with a type key steers to set_type", func(t *testing.T) {
		_, err := api.patchOps(ctx, spaceId, objectId, []any{map[string]any{"op": "set_properties", "set": map[string]any{"type": "page"}}})
		var apiErr *apiError
		require.ErrorAs(t, err, &apiErr)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Hint, "set_type")
	})

	t.Run("a dry run validates and warns, and commits nothing", func(t *testing.T) {
		var result struct {
			DryRun   bool             `json:"dry_run"`
			Warnings []map[string]any `json:"warnings"`
		}
		_, err := api.call(ctx, http.MethodPatch, objectPath, url.Values{"dry_run": {"true"}},
			map[string]any{"ops": []any{map[string]any{"op": "set_type", "type": "note"}}}, &result)
		require.NoError(t, err)
		assert.True(t, result.DryRun)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0]["message"], "not simulated")

		_, raw, err := api.getDocument(ctx, spaceId, objectId)
		require.NoError(t, err)
		assert.Equal(t, "task", readTyped(t, raw).Type, "the dry run must not have changed the type")
	})

	t.Run("a query refuses the type change permanently", func(t *testing.T) {
		var created struct {
			Id string `json:"id"`
		}
		_, err := api.call(ctx, http.MethodPost, "/v2/spaces/"+url.PathEscape(spaceId)+"/queries", nil,
			map[string]any{"name": "All tasks", "type": "task"}, &created)
		if err != nil {
			t.Skipf("no query to test against: %v", err)
		}
		_, err = api.patchOps(ctx, spaceId, created.Id, []any{map[string]any{"op": "set_type", "type": "collection"}})
		var apiErr *apiError
		require.ErrorAs(t, err, &apiErr)
		t.Logf("set_type on a query -> %d %s\n%+v", apiErr.Status, apiErr.Message, apiErr.Issues)
		assert.Equal(t, http.StatusForbidden, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Hint, "do not retry")
	})

	t.Run("the MCP wrapper's set_properties takes a type", func(t *testing.T) {
		client := wrapper.NewClient(api.baseURL, api.apiKey)
		client.HTTP = &http.Client{Timeout: 60 * time.Second}
		ts, err := newMCPToolset(ctx, wrapper.NewRunner(client, wrapper.NewMemoryStore()), wrapper.TierLarge)
		require.NoError(t, err)

		// the preceding subtests just rewrote the object several times; the
		// full-text index behind find catches up asynchronously
		searchable, waited, err := api.waitSearchable(ctx, spaceId, title, objectId, 20*time.Second)
		require.NoError(t, err)
		require.True(t, searchable, "the object never became searchable (waited %s)", waited)
		found := ts.call(ctx, "find", map[string]any{"space": spaceId, "query": title})
		t.Logf("find -> isError=%v\n%s", found.IsError, found.Text)
		require.False(t, found.IsError)
		require.True(t, strings.Contains(found.Text, "1. "), "the object must be numbered")

		out := ts.call(ctx, "set_properties", map[string]any{"object": "1", "type": "Page"})
		t.Logf("set_properties type=Page -> isError=%v\n%s", out.IsError, out.Text)
		require.False(t, out.IsError, out.Text)

		_, raw, err := api.getDocument(ctx, spaceId, objectId)
		require.NoError(t, err)
		assert.Equal(t, "page", readTyped(t, raw).Type)
	})

}
