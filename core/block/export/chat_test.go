package export

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Whole-space export used to skip both chat kinds before the writer could
// emit them, leaving index.homepage pointing at a missing document.
func TestWholeSpaceAnyBlockExportsChatsAndResolvesHomepage(t *testing.T) {
	fx := newFixture(t)
	const chatID = "synthetic-chat"
	for _, tc := range []struct {
		id, name, key string
		sbType        smartblock.SmartBlockType
	}{
		{chatID, "General", "syntheticChatKey", smartblock.SmartBlockTypeChatDerivedObject},
		{"synthetic-legacy-chat", "Legacy chat", "syntheticLegacyChatKey", smartblock.SmartBlockTypeChatObjectDeprecated},
		{"synthetic-workspace", "Chat space", "", smartblock.SmartBlockTypeWorkspace},
	} {
		details := map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String(tc.id), bundle.RelationKeyName: domain.String(tc.name),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		}
		if tc.sbType == smartblock.SmartBlockTypeWorkspace {
			details[bundle.RelationKeyHomepage] = domain.String(chatID)
		}
		fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{details})
		object := setupObject(tc.id, "", tc.sbType, details)
		st := object.NewState()
		st.SetUniqueKeyInternal(tc.key)
		object.Doc = st
		fx.picker.EXPECT().GetObject(mock.Anything, tc.id).Return(object, nil).Maybe()
		fx.sbtProvider.EXPECT().Type(spaceId, tc.id).Return(tc.sbType, nil).Maybe()
	}
	fx.picker.EXPECT().TryRemoveFromCache(mock.Anything, mock.Anything).Return(true, nil)
	exportPath, report, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
		SpaceId: spaceId, Path: t.TempDir(), Format: model.Export_AnyBlockV2, NoProgress: true,
	})
	require.NoError(t, err)
	require.Equal(t, model.ExportReport_SUCCESS, report.Status)
	assert.EqualValues(t, 3, report.Succeed)
	assert.Zero(t, report.ObjectErrors)
	for _, issue := range report.Issues {
		assert.NotEqual(t, "unresolved_target", issue.Code)
	}
	tree := readExportTree(t, exportPath)
	idx, err := anyblockjson.UnmarshalIndex([]byte(tree[anyblockjson.IndexFileName]))
	require.NoError(t, err)
	assert.Equal(t, chatID, idx.Homepage)
	if idx.Unresolved != nil {
		assert.Empty(t, idx.Unresolved.Targets)
	}
	found := map[string]bool{}
	for _, data := range tree {
		var envelope struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		}
		require.NoError(t, json.Unmarshal([]byte(data), &envelope))
		if envelope.Kind != "chat" && envelope.Kind != "chat_object" {
			continue
		}
		sbType, restored, err := anyblockjson.Unmarshal([]byte(data), anyblockjson.Options{})
		require.NoError(t, err)
		found[envelope.ID] = true
		if envelope.ID == chatID {
			assert.Equal(t, model.SmartBlockType_ChatDerivedObject, sbType)
			assert.Equal(t, "syntheticChatKey", restored.Key)
			assert.Equal(t, "General", restored.Details.Fields["name"].GetStringValue())
		} else {
			assert.Equal(t, model.SmartBlockType_ChatObjectDeprecated, sbType)
			assert.Equal(t, "syntheticLegacyChatKey", restored.Key)
		}
	}
	assert.Equal(t, map[string]bool{chatID: true, "synthetic-legacy-chat": true}, found)
}
