package anyblockjson

// kind: "chat" is the authorable name for ChatDerivedObject (§2). A chat is a
// standalone object: its identity is the envelope's "internal_key", like a type's.

import (
	"testing"

	"github.com/gogo/protobuf/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestImport_ChatKind(t *testing.T) {
	doc := `{"version": 2, "id": "chat-wiki", "kind": "chat", "internal_key": "wikiChat",
		"icon": {"format": "emoji", "emoji": "💬"},
		"properties": {"name": "Wiki"}}`
	sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)

	assert.Equal(t, model.SmartBlockType_ChatDerivedObject, sbType)
	assert.Equal(t, "wikiChat", snap.Key, "identity lives in key, like a type")
	assert.Equal(t, "chat-wiki", snap.Details.Fields["id"].GetStringValue())
	assert.Equal(t, "Wiki", snap.Details.Fields["name"].GetStringValue())
	assert.Equal(t, "💬", snap.Details.Fields["iconEmoji"].GetStringValue(),
		"the typed envelope field is where an icon is written now (§2b)")
}

func TestRoundtrip_ChatKind(t *testing.T) {
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "chat-wiki", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
		},
		Details: fields(map[string]*types.Value{
			"id":   str("chat-wiki"),
			"name": str("Wiki"),
		}),
		Key: "wikiChat",
	}
	data, err := Marshal(model.SmartBlockType_ChatDerivedObject, snapshot, testOptions())
	require.NoError(t, err)
	assert.Contains(t, string(data), `"kind": "chat"`)
	assert.NotContains(t, string(data), "chat_derived")

	sbType, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, model.SmartBlockType_ChatDerivedObject, sbType)
	assert.Equal(t, "wikiChat", back.Key)
}

// the old name is gone from the vocabulary, and the deprecated sibling kind
// stays distinct from it
func TestValidate_ChatDerivedNameRejected(t *testing.T) {
	_, _, err := Unmarshal([]byte(`{"version": 2, "kind": "chat_derived", "internal_key": "k"}`),
		Options{GenerateId: seqIds("g")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind")

	sbType, _, err := Unmarshal([]byte(`{"version": 2, "kind": "chat_object", "internal_key": "k"}`),
		Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, model.SmartBlockType_ChatObjectDeprecated, sbType)
}
