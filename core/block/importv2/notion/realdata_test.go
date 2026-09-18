package notion

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"

	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
)

// TestDecodeRecordedSearch decodes the committed cassette's real API bodies
// through the actual pass-1 and pass-2 structs, catching JSON type
// mismatches against real workspace data without hitting the network. It is
// the offline half of the schema-drift detector (TestCassetteWorkspace is
// the online half). Every /search page, every /pages and /data_sources
// object, and every /blocks response (incl. per-type block payloads) is
// covered — a mistyped field in any of them fails here, not just as a
// silent placeholder degradation in the replay.
func TestDecodeRecordedSearch(t *testing.T) {
	cas, err := cassette.Load(workspaceCassette)
	if err != nil {
		t.Skip("no cassette recorded yet")
	}

	var searchPages, pageObjects, dataSources, blockLists int
	for _, interaction := range cas.Interactions {
		switch {
		case interaction.Request.Method == "POST" && interaction.Request.URL == client.BaseURL+"/search":
			searchPages++
			decodeSearchBody(t, interaction.Response.Body)
		case interaction.Request.Method == "GET" && strings.Contains(interaction.Request.URL, "/pages/") &&
			!strings.Contains(interaction.Request.URL, "/properties/"):
			pageObjects++
			var page pageObject
			require.NoError(t, json.Unmarshal([]byte(interaction.Response.Body), &page),
				"pageObject decode: %s", interaction.Request.URL)
		case interaction.Request.Method == "GET" && strings.Contains(interaction.Request.URL, "/data_sources/"):
			dataSources++
			var database databaseObject
			require.NoError(t, json.Unmarshal([]byte(interaction.Response.Body), &database),
				"databaseObject decode: %s", interaction.Request.URL)
		case interaction.Request.Method == "GET" && strings.Contains(interaction.Request.URL, "/blocks/"):
			blockLists++
			decodeBlocksBody(t, interaction.Request.URL, interaction.Response.Body)
		}
	}
	require.Positive(t, searchPages, "cassette has no /search interaction")
	require.Positive(t, pageObjects, "cassette has no /pages interaction")
	require.Positive(t, dataSources, "cassette has no /data_sources interaction")
	require.Positive(t, blockLists, "cassette has no /blocks interaction")
	t.Logf("decoded %d search pages, %d pages, %d data sources, %d block lists",
		searchPages, pageObjects, dataSources, blockLists)
}

func decodeSearchBody(t *testing.T, body string) {
	t.Helper()

	// Pass 1: the whole response must decode through searchResponse.
	var resp searchResponse
	require.NoError(t, json.Unmarshal([]byte(body), &resp), "searchResponse decode")
	require.NotEmpty(t, resp.Results)

	// Pass 2: every page property value must decode through propertyValue,
	// every data-source schema property through propertySchema.
	var generic struct {
		Results []struct {
			Object     string                     `json:"object"`
			Id         string                     `json:"id"`
			Properties map[string]json.RawMessage `json:"properties"`
			Icon       json.RawMessage            `json:"icon"`
			Cover      json.RawMessage            `json:"cover"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &generic))

	for _, r := range generic.Results {
		for key, raw := range r.Properties {
			if r.Object == "page" {
				var pv propertyValue
				require.NoError(t, json.Unmarshal(raw, &pv),
					"page %s property %q: %s", r.Id, key, raw)
			} else {
				var ps propertySchema
				require.NoError(t, json.Unmarshal(raw, &ps),
					"%s %s property %q: %s", r.Object, r.Id, key, raw)
			}
		}
		if len(r.Icon) > 0 && string(r.Icon) != "null" {
			var icon iconValue
			require.NoError(t, json.Unmarshal(r.Icon, &icon), "%s %s icon: %s", r.Object, r.Id, r.Icon)
		}
		if len(r.Cover) > 0 && string(r.Cover) != "null" {
			var cover fileValue
			require.NoError(t, json.Unmarshal(r.Cover, &cover), "%s %s cover: %s", r.Object, r.Id, r.Cover)
		}
	}
}

// decodeBlocksBody replays the per-type decodes mapBlock would perform, so a
// payload-shape drift fails loudly instead of degrading every real block to
// an unsupported placeholder that the replay test tolerates.
func decodeBlocksBody(t *testing.T, url, body string) {
	t.Helper()
	var resp blockListResponse
	require.NoError(t, json.Unmarshal([]byte(body), &resp), "blockListResponse decode: %s", url)
	for i := range resp.Results {
		block := &resp.Results[i]
		var target any
		switch block.Type {
		case "paragraph", "heading_1", "heading_2", "heading_3", "heading_4",
			"bulleted_list_item", "numbered_list_item", "to_do", "toggle",
			"quote", "callout", "code", "equation", "embed", "bookmark", "link_preview":
			target = &textPayload{}
		case "image", "pdf", "file", "video", "audio":
			target = &filePayload{}
		case "child_page", "child_database":
			target = &childEntityPayload{}
		case "table":
			target = &tablePayload{}
		case "table_row":
			target = &tableRowPayload{}
		case "link_to_page":
			target = &linkToPagePayload{}
		default:
			continue
		}
		require.NoError(t, block.decode(target), "block %s (%s) payload decode", block.Id, block.Type)
	}
}
