package anyblockjson

// kind: "chat" is the authorable name for ChatDerivedObject (§2). A chat is a
// standalone object: its identity is the envelope's "key", like a type's.

import (
	"testing"

	"github.com/gogo/protobuf/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestImport_ChatKind(t *testing.T) {
	doc := `{"version": 1, "id": "chat-wiki", "kind": "chat", "key": "wikiChat",
		"properties": {"name": "Wiki", "iconEmoji": "💬"}}`
	sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)

	assert.Equal(t, model.SmartBlockType_ChatDerivedObject, sbType)
	assert.Equal(t, "wikiChat", snap.Key, "identity lives in key, like a type")
	assert.Equal(t, "chat-wiki", snap.Details.Fields["id"].GetStringValue())
	assert.Equal(t, "Wiki", snap.Details.Fields["name"].GetStringValue())
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
	assert.NotContains(t, string(data), "chatDerived")

	sbType, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, model.SmartBlockType_ChatDerivedObject, sbType)
	assert.Equal(t, "wikiChat", back.Key)
}

// the old name is gone from the vocabulary, and the deprecated sibling kind
// stays distinct from it
func TestValidate_ChatDerivedNameRejected(t *testing.T) {
	_, _, err := Unmarshal([]byte(`{"version": 1, "kind": "chatDerived", "key": "k"}`),
		Options{GenerateId: seqIds("g")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind")

	sbType, _, err := Unmarshal([]byte(`{"version": 1, "kind": "chatObject", "key": "k"}`),
		Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, model.SmartBlockType_ChatObjectDeprecated, sbType)
}
