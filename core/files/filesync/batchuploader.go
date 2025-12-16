package filesync

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
)

func (s *fileSync) runBatchUploader() {
	for {
		select {
		case <-s.loopCtx.Done():
			return
		case req := <-s.requestsCh:

			err := s.rpcStore.AddToFileMany(s.loopCtx, req.req)
			go func(req blockPushManyRequest, err error) {
				if err != nil {
					s.onBatchUploadError(s.loopCtx, req, err)
				} else {
					s.onBatchUploaded(s.loopCtx, req)
				}
			}(req, err)
		}
	}
}

func (s *fileSync) addToLimitedQueue(objectId string) error {
	return s.process(objectId, func(exists bool, info FileInfo) (FileInfo, bool, error) {
		if !exists {
			return FileInfo{}, false, nil
		}

		err := s.handleLimitReached(s.loopCtx, info)
		if err != nil {
			info.State = FileStatePendingUpload
			info = info.Reschedule()
			return info, true, err
		}
		info.State = FileStateLimited
		return info, true, nil
	})
}

func (s *fileSync) processFileUploading(ctx context.Context, it FileInfo) (FileInfo, error) {
	if len(it.CidsToUpload) == 0 {
		space, err := s.limitManager.getSpace(ctx, it.SpaceId)
		if err != nil {
			return it, fmt.Errorf("get space limits: %w", err)
		}
		space.markFileUploaded(it.Key())

		err = s.updateStatus(it, filesyncstatus.Synced)
		if err != nil {
			return it, err
		}
		it.State = FileStateDone
		return it, nil
	}

	return it, nil
}

func (s *fileSync) addToRetryUploadingQueue(objectId string) error {
	return s.process(objectId, func(exists bool, info FileInfo) (FileInfo, bool, error) {
		if !exists {
			return FileInfo{}, false, nil
		}

		info.State = FileStatePendingUpload
		info = info.Reschedule()
		return info, true, nil
	})
}

func (s *fileSync) updateUploadedCids(objectId string, cids []cid.Cid) error {
	return s.process(objectId, func(exists bool, info FileInfo) (FileInfo, bool, error) {
		if !exists {
			return FileInfo{}, false, nil
		}

		// If deletion is pending, it will be deleted soon
		if info.State == FileStatePendingDeletion {
			return FileInfo{}, false, nil
		}

		// If it was deleted, delete again to undo uploaded blocks
		if info.State == FileStateDeleted {
			err := s.rpcStore.DeleteFiles(s.loopCtx, info.SpaceId, info.FileId)
			// Enqueue deletion if we can't delete it right away
			if err != nil {
				info.State = FileStatePendingDeletion
				return info, true, err
			}
			return info, true, nil
		}

		for _, c := range cids {
			delete(info.CidsToUpload, c)
		}
		next, err := s.processFileUploading(s.loopCtx, info)
		return next, true, err
	})
}

func (s *fileSync) onBatchUploadError(ctx context.Context, req blockPushManyRequest, err error) {
	if isNodeLimitReachedError(err) {
		for _, fb := range req.req.FileBlocks {
			objectId := req.fileIdToObjectId[fb.FileId]
			err := s.addToLimitedQueue(objectId)
			if err != nil {
				log.Error("handle batch upload error: add to limited queue", zap.Error(err))
			}
		}
	} else {
		log.Error("add to file many:", zap.Error(err))
		for _, fb := range req.req.FileBlocks {
			objectId := req.fileIdToObjectId[fb.FileId]
			err := s.addToRetryUploadingQueue(objectId)
			if err != nil {
				log.Error("handle batch upload error: add to retry queue", zap.Error(err))
			}
		}
	}
}

func (s *fileSync) onBatchUploaded(ctx context.Context, req blockPushManyRequest) {
	for _, fb := range req.req.FileBlocks {
		cids := make([]cid.Cid, 0, len(fb.Blocks))
		for _, b := range fb.Blocks {
			c, err := cid.Cast(b.Cid)
			if err != nil {
				log.Error("failed to parse block cid", zap.Error(err))
			}
			cids = append(cids, c)
		}
		objectId := req.fileIdToObjectId[fb.FileId]
		err := s.updateUploadedCids(objectId, cids)
		if err != nil {
			log.Error("handle batch upload: update uploaded cids", zap.Error(err))
		}
	}
}
