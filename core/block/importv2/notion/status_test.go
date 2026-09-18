package notion

import (
	"testing"

	"github.com/stretchr/testify/assert"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
)

// The converter half of the producer seam for the flagship driver: the
// plan step announces ANALYZING, and every entity announces itself as the
// current item — in EMISSION order, never in prefetch order (the prefetch
// pipeline runs up to six pages ahead of what the user is being shown).

func TestNotionAnnouncesAnalysisAndItems(t *testing.T) {
	t.Run("the plan step brackets itself, and items follow stub order", func(t *testing.T) {
		// given: one data source and two workspace pages
		search := `{"results":[
			{"object":"data_source","id":"db1","parent":{"type":"database_id","database_id":"realdb1"},
			 "database_parent":{"type":"workspace","workspace":true},
			 "title":[{"plain_text":"Tasks","type":"text"}]},
			{"object":"page","id":"p1","parent":{"type":"workspace","workspace":true},
			 "properties":{"Name":{"type":"title","title":[{"plain_text":"Alpha","type":"text"}]}}},
			{"object":"page","id":"p2","parent":{"type":"workspace","workspace":true},
			 "properties":{"Name":{"type":"title","title":[{"plain_text":"Beta","type":"text"}]}}}
		],"has_more":false,"next_cursor":null}`
		routes := map[string]apiResponse{
			"GET /data_sources/db1": {body: `{"id":"db1","title":[{"plain_text":"Tasks","type":"text"}],
				"properties":{"Name":{"id":"title","type":"title","name":"Name"}}}`},
			"POST /data_sources/db1/query": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
			"GET /pages/p1": {body: `{"id":"p1","parent":{"type":"workspace","workspace":true},
				"properties":{"Name":{"type":"title","title":[{"plain_text":"Alpha","type":"text"}]}}}`},
			"GET /pages/p2": {body: `{"id":"p2","parent":{"type":"workspace","workspace":true},
				"properties":{"Name":{"type":"title","title":[{"plain_text":"Beta","type":"text"}]}}}`},
			"GET /blocks/p1/children": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
			"GET /blocks/p2/children": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
		}
		converter := recoveryConverter(t, recoveryWorkspace(t, search, routes))

		// when
		sink := driveConverter(t, converter)

		// then: the analysis bracket comes first and closes
		assert.Equal(t, []importv2.Phase{importv2.PhaseAnalyzing, importv2.PhaseFetching}, sink.phases)

		// and: the database, then the pages in stub order
		assert.Equal(t, []importv2.DisplayText{"Tasks", "Alpha", "Beta"}, sink.items)
	})
}
