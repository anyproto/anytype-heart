package storeresolver

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/compose"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchivedTypeExportIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		hidden    bool
	}{
		{name: "custom", key: "SYNTHETIC_archived_type"},
		{name: "hidden bundled chat", key: "chat", hidden: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := "SYNTHETIC_archived_type_object"
			row := typeRow(id, tc.key, "Archived type")
			row[bundle.RelationKeyIsArchived] = domain.Bool(true)
			row[bundle.RelationKeyIsHidden] = domain.Bool(tc.hidden)
			resolver := vocabFixture(t, row)
			_, err := compose.BuildPlan(resolver.Options(), []compose.DocMeta{{Id: id, SbType: model.SmartBlockType_STType, Key: tc.key}})
			require.NoError(t, err, "exporting an archived type must resolve its document id and references consistently")
			key, ok := resolver.TypeKeyById(id)
			require.True(t, ok)
			assert.Equal(t, tc.key, key)
			got, ok := resolver.TypeIdByKey(tc.key)
			require.True(t, ok)
			assert.Equal(t, id, got)
		})
	}
}

func TestArchivedTypeDoesNotOwnLiveName(t *testing.T) {
	archived := typeRow("SYNTHETIC_archived_object", "SYNTHETIC_archived_key", "SYNTHETIC project type")
	archived[bundle.RelationKeyIsArchived] = domain.Bool(true)
	live := typeRow("SYNTHETIC_live_object", "SYNTHETIC_live_key", "SYNTHETIC project type")
	resolver := vocabFixture(t, archived, live)
	key, ok := resolver.TypeKey("SYNTHETIC project type")
	require.True(t, ok)
	assert.Equal(t, "SYNTHETIC_live_key", key)
	key, ok = resolver.TypeKeyById("SYNTHETIC_archived_object")
	require.True(t, ok)
	assert.Equal(t, "SYNTHETIC_archived_key", key)
}

func TestLiveTypePreferredOverArchivedDuplicateKey(t *testing.T) {
	archived := typeRow("SYNTHETIC_archived_object", "SYNTHETIC_shared_key", "Archived")
	archived[bundle.RelationKeyIsArchived] = domain.Bool(true)
	live := typeRow("SYNTHETIC_live_object", "SYNTHETIC_shared_key", "Live")
	resolver := vocabFixture(t, archived, live)
	id, ok := resolver.TypeIdByKey("SYNTHETIC_shared_key")
	require.True(t, ok)
	assert.Equal(t, "SYNTHETIC_live_object", id)
	key, ok := resolver.TypeKeyById("SYNTHETIC_archived_object")
	require.True(t, ok)
	assert.Equal(t, "SYNTHETIC_shared_key", key)
}
