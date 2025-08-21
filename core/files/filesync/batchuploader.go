package filesync

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/util/timeid"
)

func (s *fileSync) runUploader() {
	for {
		select {
		case <-s.loopCtx.Done():
			return
		case req := <-s.requestsCh:
			err := s.rpcStore.AddToFileMany(s.loopCtx, req.req)
			if err != nil {
				s.onBatchUploadError(s.loopCtx, req, err)
			} else {
				s.onBatchUploaded(s.loopCtx, req)
			}
		}
	}
}

func (s *fileSync) onBatchUploadError(ctx context.Context, req blockPushManyRequest, err error) {
	if isNodeLimitReachedError(err) {
		for _, fb := range req.req.FileBlocks {
			objectId := req.fileIdToObjectId[fb.FileId]
			err = s.addToLimitedUploadingQueue(ctx, &QueueItem{
				SpaceId:     fb.SpaceId,
				ObjectId:    objectId,
				FileId:      domain.FileId(fb.FileId),
				Timestamp:   float64(time.Now().UnixMilli()),
				AddedByUser: false,
				Imported:    false,
			})
			if err != nil {
				log.Error("batch uploader: add to limited queue", zap.Error(err))
			}
		}
	}
	if err != nil {
		log.Error("add to file many:", zap.Error(err))
		for _, fb := range req.req.FileBlocks {
			objectId := req.fileIdToObjectId[fb.FileId]
			err = s.addToRetryUploadingQueue(&QueueItem{
				SpaceId:     fb.SpaceId,
				ObjectId:    objectId,
				FileId:      domain.FileId(fb.FileId),
				Timestamp:   float64(time.Now().UnixMilli()),
				AddedByUser: false,
				Imported:    false,
			})
			if err != nil {
				log.Error("batch uploader: add to retry queue", zap.Error(err))
			}
		}
	}
}

func (s *fileSync) onBatchUploaded(ctx context.Context, req blockPushManyRequest) {
	for _, fb := range req.req.FileBlocks {
		for _, b := range fb.Blocks {
			c, err := cid.Cast(b.Cid)
			if err != nil {
				log.Error("failed to parse block cid", zap.Error(err))
			} else {
				s.uploadStatusIndex.remove(fb.FileId, c, func(fileObjectId string, fullFileId domain.FullFileId) error {
					err := s.addToStatusUpdateQueue(&statusUpdateItem{
						FileObjectId: fileObjectId,
						FileId:       fullFileId.FileId.String(),
						SpaceId:      fullFileId.SpaceId,
						Timestamp:    timeid.NewNano(),
						Status:       int(filesyncstatus.Synced),
					})
					if err != nil {
						return fmt.Errorf("add to status update queue: %w", err)
					}

					err = s.pendingUploads.Delete(ctx, fileObjectId)
					if err != nil {
						return fmt.Errorf("delete pending uploads: %w", err)
					}
					return nil
				})
			}
		}
	}
}
