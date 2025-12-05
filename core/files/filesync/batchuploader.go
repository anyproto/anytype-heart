package filesync

import (
	"context"
	"time"

	"github.com/ipfs/go-cid"
	"go.uber.org/zap"
)

func (s *fileSync) runBatchUploader() {
	for {
		select {
		case <-s.loopCtx.Done():
			return
		case req := <-s.requestsCh:

			err := s.rpcStore.AddToFileMany(s.loopCtx, req.req)
			go func() {
				if err != nil {
					s.onBatchUploadError(s.loopCtx, req, err)
				} else {
					s.onBatchUploaded(s.loopCtx, req)
				}
			}()
		}
	}
}

func (s *fileSync) addToLimitedQueue(objectId string) error {
	return s.process(objectId, func(exists bool, info FileInfo) (FileInfo, error) {
		if !exists {
			return FileInfo{}, nil
		}

		err := s.handleLimitReached(s.loopCtx, info)
		if err != nil {
			return FileInfo{}, err
		}
		info.State = FileStateLimited
		return info, nil
	})
}

func (s *fileSync) addToRetryUploadingQueue(objectId string) error {
	return s.process(objectId, func(exists bool, info FileInfo) (FileInfo, error) {
		if !exists {
			return FileInfo{}, nil
		}

		info.State = FileStatePendingUpload
		// TODO add jitter
		info.ScheduledAt = time.Now().Add(1 * time.Minute)
		return info, nil
	})
}

func (s *fileSync) updateUploadedCids(objectId string, cids []cid.Cid) error {
	return s.process(objectId, func(exists bool, info FileInfo) (FileInfo, error) {
		if !exists {
			return FileInfo{}, nil
		}

		// If deletion is pending, it will be deleted soon
		if info.State == FileStatePendingDeletion {
			return FileInfo{}, nil
		}

		// If it was deleted, delete again to undo uploaded blocks
		if info.State == FileStateDeleted {
			err := s.rpcStore.DeleteFiles(s.loopCtx, info.SpaceId, info.FileId)
			// Enqueue deletion if we can't delete it right away
			if err != nil {
				info.State = FileStatePendingDeletion
				return info, err
			}
			return info, nil
		}

		for _, c := range cids {
			delete(info.CidsToUpload, c)
		}
		next, err := s.processFileUploading(s.loopCtx, info)
		return next, err
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
