package storeresolver

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Attribution references remain bare ids regardless of whether the index can
// name the participant. Exercise the Heart bridge with the store resolvers.
func TestExportParticipantReferences(t *testing.T) {
	const participantId = "_participant_bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq_30afw2fe3tvff_AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA"
	for _, tc := range []struct {
		name    string
		present bool
		label   string
	}{
		{"named", true, "Roman"},
		{"unnamed", true, ""},
		{"absent", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index := spaceindex.NewStoreFixture(t)
			if tc.present {
				index.AddObjects(t, []spaceindex.TestObject{{
					bundle.RelationKeyId:             domain.String(participantId),
					bundle.RelationKeyName:           domain.String(tc.label),
					bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
				}})
			}
			opts := New(index).Options()
			spaceId, identity, err := domain.ParseParticipantId(participantId)
			require.NoError(t, err)
			opts.SpaceId = spaceId
			snapshot := &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					"creator":        {Kind: &types.Value_StringValue{StringValue: participantId}},
					"lastModifiedBy": {Kind: &types.Value_StringValue{StringValue: participantId}},
				}},
			}

			data, err := anyblockjson.Marshal(model.SmartBlockType_Page, snapshot, opts)
			require.NoError(t, err)
			var document struct {
				Properties map[string]any `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(data, &document))
			assert.Equal(t, "participant-"+identity, document.Properties["Created by"])
			assert.Equal(t, "participant-"+identity, document.Properties["Last modified by"])
		})
	}
}
