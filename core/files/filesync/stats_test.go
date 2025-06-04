package filesync

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

func TestSpaceUsageUpdate(t *testing.T) {
	const limit = 1024 * 1024 * 1024
	fx := newFixture(t, limit)
	defer fx.Finish(t)

	nodeUsage, err := fx.getAndUpdateNodeUsage(ctx)
	require.NoError(t, err)

	assert.Equal(t, 0, nodeUsage.TotalBytesUsage)
	assert.Equal(t, 0, nodeUsage.TotalCidsCount)
	assert.Equal(t, limit, nodeUsage.AccountBytesLimit)
	assert.Equal(t, uint64(limit), nodeUsage.BytesLeft)
	assert.Len(t, nodeUsage.Spaces, 0)

	var fileSize1 uint64
	t.Run("one file uploaded", func(t *testing.T) {
		// Upload file
		// Add file to local DAG
		fileId, fileNode := fx.givenFileAddedToDAG(t)
		spaceId := "space1"
		fileSize1, _ = fileNode.Size()

		// Add file to upload queue
		fx.givenFileUploaded(t, spaceId, fileId)

		nodeUsage, err = fx.getAndUpdateNodeUsage(ctx)
		require.NoError(t, err)

		assert.Equal(t, fileSize1, uint64(nodeUsage.TotalBytesUsage))
		assert.Equal(t, limit-fileSize1, nodeUsage.BytesLeft)
		assert.True(t, nodeUsage.TotalCidsCount > 0)
		assert.Len(t, nodeUsage.Spaces, 1)

		spaceUsage := nodeUsage.GetSpaceUsage(spaceId)
		assert.Equal(t, fileSize1, uint64(spaceUsage.TotalBytesUsage))
		assert.Equal(t, fileSize1, uint64(spaceUsage.SpaceBytesUsage))
	})

	var fileSize2 uint64
	t.Run("two files uploaded", func(t *testing.T) {
		// Upload another file
		// Add file to local DAG
		fileId, fileNode := fx.givenFileAddedToDAG(t)
		spaceId := "space2"
		fileSize2, _ = fileNode.Size()

		// Add file to upload queue
		fx.givenFileUploaded(t, spaceId, fileId)

		nodeUsage, err = fx.getAndUpdateNodeUsage(ctx)
		require.NoError(t, err)

		assert.Equal(t, fileSize1+fileSize2, uint64(nodeUsage.TotalBytesUsage))
		assert.Equal(t, limit-fileSize1-fileSize2, nodeUsage.BytesLeft)
		assert.True(t, nodeUsage.TotalCidsCount > 0)
		assert.Len(t, nodeUsage.Spaces, 2)

		spaceUsage := nodeUsage.GetSpaceUsage(spaceId)
		assert.Equal(t, fileSize1+fileSize2, uint64(spaceUsage.TotalBytesUsage))
		assert.Equal(t, fileSize2, uint64(spaceUsage.SpaceBytesUsage))
	})

	t.Run("update limit", func(t *testing.T) {
		fx.rpcStore.SetLimit(limit * 10)

		err = fx.UpdateNodeUsage(ctx)
		require.NoError(t, err)

		// Event is expected to be sent
	})

	t.Run("events sent", func(t *testing.T) {
		fx.eventsLock.Lock()
		defer fx.eventsLock.Unlock()

		wantEvents := []*pb.Event{
			makeLimitUpdatedEvent(limit),
			makeSpaceUsageEvent("space1", fileSize1),
			makeSpaceUsageEvent("space2", fileSize2),
			makeLimitUpdatedEvent(limit * 10),
		}

		if !assert.Equal(t, wantEvents, fx.events) {
			m := json.NewEncoder(os.Stdout)
			m.SetIndent("", "  ")
			m.Encode(wantEvents)
			fmt.Println("---")
			m.Encode(fx.events)
		}
	})
}
