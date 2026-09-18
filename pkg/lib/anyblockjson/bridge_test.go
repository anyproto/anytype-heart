package anyblockjson

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestFileRemoteAlwaysIncluded(t *testing.T) {
	info := &model.FileInfo{
		FileId: "bafybeiaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EncryptionKeys: []*model.FileEncryptionKey{{
			Path: "/0/", Key: "synthetic-test-key",
		}},
	}
	for _, opts := range []Options{{}, {OmitIds: true, CompactBlockLabels: true}} {
		data, err := Marshal(model.SmartBlockType_FileObject, &model.SmartBlockSnapshotBase{FileInfo: info}, opts)
		require.NoError(t, err)
		var document struct {
			FileRemote string `json:"file_remote"`
		}
		require.NoError(t, json.Unmarshal(data, &document))
		require.NotEmpty(t, document.FileRemote)

		_, restored, err := Unmarshal(data, Options{})
		require.NoError(t, err)
		assert.Equal(t, info, restored.FileInfo)
	}
}
