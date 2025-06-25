package filesync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

func TestCancelDeletion(t *testing.T) {
	s := newFixtureNotStarted(t, 100000000)
	err := s.Init(s.a)
	require.NoError(t, err)

	testObjectId1 := "objectId1"
	testFileId1 := domain.FullFileId{SpaceId: "spaceId", FileId: "bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"}

	testObjectId2 := "objectId2"
	testFileId2 := domain.FullFileId{SpaceId: "spaceId", FileId: "bafybeiasl27gslws4hpvzufm467zjhxb3klodj53rt6dpola67bmvep3x4"}

	s.loopCtx = context.Background()

	err = s.deletionQueue.Add(&deletionQueueItem{
		ObjectId: testObjectId1,
		SpaceId:  testFileId1.SpaceId,
		FileId:   testFileId1.FileId,
	})
	require.NoError(t, err)

	err = s.retryDeletionQueue.Add(&deletionQueueItem{
		ObjectId: testObjectId2,
		SpaceId:  testFileId2.SpaceId,
		FileId:   testFileId2.FileId,
	})
	require.NoError(t, err)

	s.deletionQueue.Run()
	s.retryDeletionQueue.Run()

	err = s.CancelDeletion(testObjectId1, testFileId1)
	require.NoError(t, err)

	err = s.CancelDeletion(testObjectId2, testFileId2)
	require.NoError(t, err)

	assert.Zero(t, s.deletionQueue.Len())
	assert.Zero(t, s.retryDeletionQueue.Len())
}
