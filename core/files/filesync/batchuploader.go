package filesync

import (
	"time"

	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
)

func (s *fileSync) runUploader() {
	// TODO Handle pending uploads
	for {
		select {
		case <-s.loopCtx.Done():
			return
		case req := <-s.requestsCh:
			err := s.rpcStore.AddToFileMany(s.loopCtx, req.req)
			if isNodeLimitReachedError(err) {
				for _, fb := range req.req.FileBlocks {
					objectId := req.fileIdToObjectId[fb.FileId]
					err = s.limitedUploadingQueue.Add(&QueueItem{
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
					err = s.retryUploadingQueue.Add(&QueueItem{
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
			} else {
				for _, fb := range req.req.FileBlocks {
					for _, b := range fb.Blocks {
						c, err := cid.Cast(b.Cid)
						if err != nil {
							log.Error("failed to parse block cid", zap.Error(err))
						} else {
							s.uploadStatusIndex.remove(fb.FileId, c)
						}
					}
				}
			}
		}
	}
}
