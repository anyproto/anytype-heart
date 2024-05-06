package reconciler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

func TestQueueItemMarshalUnmarshal(t *testing.T) {
	item := queueItem{
		ObjectId: "objectId",
		FileId:   domain.FullFileId{SpaceId: "spaceId", FileId: "fileId"},
	}

	raw, err := json.Marshal(item)
	require.NoError(t, err)

	var got queueItem
	err = json.Unmarshal(raw, &got)
	require.NoError(t, err)

	assert.Equal(t, item, got)
}
