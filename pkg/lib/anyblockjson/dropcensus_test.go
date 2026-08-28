package anyblockjson

// dropcensus_test.go — the term census must not reserve a spelling for a
// detail key the properties emit is going to DROP.
//
// A dropped key is written nowhere, so a generation-2 census cannot hold
// it. If generation 1 let it contest a spelling anyway, the rival it
// suffixed un-suffixes on the next export and the round trip stops being a
// fixpoint. The attribution keys were the only population modelled; four
// more drop inside buildProperties, and each gets an arm here.
//
// Every arm has the same shape: a bundled key that WILL be dropped, a
// custom property carrying the same display name, and the assertion that
// the custom property keeps its plain name in generation 1 and in
// generation 2 — byte-identical output across the round trip.

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// droppedKeyCase is one drop population: the snapshot that carries both the
// dropped key and its custom name-twin, plus what the twin must be spelled.
type droppedKeyCase struct {
	name        string
	sbType      model.SmartBlockType
	droppedKey  string
	sharedName  string
	extraFields map[string]*types.Value
	// writtenVerbatim marks the one population export DOES write — the
	// attribution keys, whose value is written under the stored key and
	// then dropped by import. The two generations differ by that one line
	// and cannot be byte-compared; every other population is written
	// nowhere, so its two generations must be identical.
	writtenVerbatim bool
}

func TestDroppedPropertyKeyNeverContestsASpelling(t *testing.T) {
	const twin = "6a7663db61fab21cd4b90aa1"

	cases := []droppedKeyCase{{
		// DroppedEmptySystemProperty: `isHidden: false` says nothing a
		// reader could act on, so it is not written
		name:       "an empty system-stamped flag",
		sbType:     model.SmartBlockType_Page,
		droppedKey: "isHidden",
		sharedName: "Hidden",
		extraFields: map[string]*types.Value{
			"isHidden": pbtypes.Bool(false),
		},
	}, {
		// typeProvenanceKeys: a type document does not carry its own
		// install provenance
		name:       "a type document's install provenance",
		sbType:     model.SmartBlockType_STType,
		droppedKey: "origin",
		sharedName: "Origin",
		extraFields: map[string]*types.Value{
			"origin":            pbtypes.Int64(3),
			"uniqueKey":         pbtypes.String("ot-custom"),
			"type":              pbtypes.String("objectType"),
			"layout":            pbtypes.Int64(int64(model.ObjectType_objectType)),
			"recommendedLayout": pbtypes.Int64(int64(model.ObjectType_basic)),
		},
	}, {
		// DroppedParticipantProvenanceKey: a participant's createdDate is a
		// load timestamp, re-stamped on every cold build
		name:       "a participant's load timestamp",
		sbType:     model.SmartBlockType_Participant,
		droppedKey: "createdDate",
		sharedName: "Creation date",
		extraFields: map[string]*types.Value{
			"createdDate": pbtypes.Int64(1700000000),
			"identity":    pbtypes.String("A5qTLyde3S1q9NRyFeSeN6UWwa6VwwXEJbMACJwMfez3BGVD"),
		},
	}, {
		// a name-over-number key holding a string its vocabulary cannot
		// name: there is no way to write it, so it is dropped
		name:       "an unnameable named-enum value",
		sbType:     model.SmartBlockType_Page,
		droppedKey: "layoutAlign",
		sharedName: "Layout align",
		extraFields: map[string]*types.Value{
			"layoutAlign": pbtypes.String("not a name this vocabulary holds"),
		},
	}, {
		// the population that was already modelled, kept here so all five
		// are pinned in one place. This one is WRITTEN — under its stored
		// key — and dropped on the way back in.
		name:       "an attribution key",
		sbType:     model.SmartBlockType_Page,
		droppedKey: "creator",
		sharedName: "Created by",
		extraFields: map[string]*types.Value{
			"creator": pbtypes.String("_participant_a_b_A5qTLyde3S1q9NRyFeSeN6UWwa6VwwXEJbMACJwMfez3BGVD"),
		},
		writtenVerbatim: true,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// given — the dropped key's name-twin is a live custom property
			vocab := nameVocab{names: map[string]string{twin: tc.sharedName}}
			fields := map[string]*types.Value{
				"id":   pbtypes.String("o1"),
				twin:   pbtypes.String("the twin's value"),
				"name": pbtypes.String("An object"),
			}
			for k, v := range tc.extraFields {
				fields[k] = v
			}
			snap := &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: fields}}
			opts := Options{Keys: vocab, SpaceId: "a.b"}

			// when
			data, err := Marshal(tc.sbType, snap, opts)
			require.NoError(t, err)

			// then — the twin keeps the plain name it will re-derive once
			// the dropped key is gone
			require.NoError(t, Validate(data), "I1:\n%s", data)
			var doc struct {
				Properties map[string]any `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(data, &doc))
			assert.Equal(t, "the twin's value", doc.Properties[tc.sharedName],
				"a key the emit drops must not degrade the rival it contests")
			if tc.writtenVerbatim {
				assert.Contains(t, doc.Properties, tc.droppedKey,
					"a yielding claimant is written under its own stored key")
			} else {
				assert.NotContains(t, doc.Properties, tc.droppedKey,
					"the dropped key claims no spelling of its own")
			}

			// the fixpoint: generation 2 sees no dropped key at all, so a
			// spelling it had contested would move
			_, back, err := Unmarshal(data, opts)
			require.NoError(t, err)
			again, err := Marshal(tc.sbType, back, opts)
			require.NoError(t, err)
			var doc2 struct {
				Properties map[string]any `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(again, &doc2))
			assert.Equal(t, "the twin's value", doc2.Properties[tc.sharedName],
				"generation 2 re-derives the same spelling — the fixpoint the rule exists for")
			if !tc.writtenVerbatim {
				assert.Equal(t, string(data), string(again),
					"a key written nowhere leaves the two generations byte-identical")
			}
		})
	}
}
