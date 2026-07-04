package notion

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"

	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
)

// TestDecodeRecordedSearch decodes the committed cassette's real /search
// response through the actual pass-1 and pass-2 structs, catching JSON
// type-mismatches against real workspace data without hitting the network.
// It is the offline half of the schema-drift detector (TestCassetteWorkspace
// is the online half).
func TestDecodeRecordedSearch(t *testing.T) {
	cas, err := cassette.Load(workspaceCassette)
	if err != nil {
		t.Skip("no cassette recorded yet")
	}
	var searchBody string
	for _, i := range cas.Interactions {
		if i.Request.URL == client.BaseURL+"/search" || i.Request.Method == "POST" {
			searchBody = i.Response.Body
			break
		}
	}
	require.NotEmpty(t, searchBody, "cassette has no /search interaction")

	// Pass 1: the whole response must decode through searchResponse.
	var resp searchResponse
	require.NoError(t, json.Unmarshal([]byte(searchBody), &resp), "searchResponse decode")
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
	require.NoError(t, json.Unmarshal([]byte(searchBody), &generic))

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
