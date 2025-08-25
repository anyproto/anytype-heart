package filesync

//
// func TestDeleteFile(t *testing.T) {
// 	t.Run("if all is ok, delete file", func(t *testing.T) {
// 		fx := newFixture(t, 1024*1024*1024)
// 		defer fx.Finish(t)
// 		spaceId := "spaceId"
//
// 		fileId, _ := fx.givenFileAddedToDAG(t)
// 		fx.givenFileUploaded(t, spaceId, fileId)
//
// 		err := fx.DeleteFile("objectId", domain.FullFileId{SpaceId: spaceId, FileId: fileId})
// 		require.NoError(t, err)
//
// 		fx.waitEmptyQueue(t, fx.deletionQueue, time.Second*1)
//
// 		resp, err := fx.rpcStore.FilesInfo(ctx, spaceId, fileId)
// 		require.NoError(t, err)
// 		require.Empty(t, resp)
// 	})
//
// 	t.Run("with error while deleting, add to retry queue", func(t *testing.T) {
// 		fx := newFixture(t, 1024*1024*1024)
// 		defer fx.Finish(t)
// 		spaceId := "spaceId"
//
// 		testFileId := domain.FileId("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")
//
// 		// Just try to delete missing file, in-memory RPC store will return error
// 		err := fx.DeleteFile("objectId", domain.FullFileId{SpaceId: spaceId, FileId: testFileId})
// 		require.NoError(t, err)
//
// 		fx.waitEmptyQueue(t, fx.deletionQueue, time.Second*1)
//
// 		fx.waitCondition(t, 100*time.Millisecond, func() bool {
// 			return fx.retryDeletionQueue.Len() == 1 || fx.retryDeletionQueue.NumProcessedItems() > 0
// 		})
// 	})
// }
