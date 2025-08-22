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
			if err != nil {
				s.onBatchUploadError(s.loopCtx, req, err)
			} else {
				s.onBatchUploaded(s.loopCtx, req)
			}
		}
	}
}

func (s *fileSync) addToLimitedQueue(objectId string) {
	s.stateProcessor.process(objectId, func(exists bool, info FileInfo) (ProcessAction, FileInfo, error) {
		if !exists {
			return ProcessActionNone, FileInfo{}, nil
		}

		return ProcessActionUpdate, info.ToLimitReached(), nil
	})
}

func (s *fileSync) addToRetryUploadingQueue(objectId string) {
	s.stateProcessor.process(objectId, func(exists bool, info FileInfo) (ProcessAction, FileInfo, error) {
		if !exists {
			return ProcessActionNone, FileInfo{}, nil
		}

		info.HandledAt = time.Now()
		return ProcessActionUpdate, info, nil
	})
}

func (s *fileSync) updateUploadedCids(objectId string, cids []cid.Cid) {
	s.stateProcessor.process(objectId, func(exists bool, info FileInfo) (ProcessAction, FileInfo, error) {
		if !exists {
			return ProcessActionNone, FileInfo{}, nil
		}
		for _, c := range cids {
			delete(info.CidsToUpload, c)
		}
		next, err := s.processFileUploading(s.loopCtx, info)
		return ProcessActionUpdate, next, err
	})
}

func (s *fileSync) onBatchUploadError(ctx context.Context, req blockPushManyRequest, err error) {
	if isNodeLimitReachedError(err) {
		for _, fb := range req.req.FileBlocks {
			objectId := req.fileIdToObjectId[fb.FileId]
			s.addToLimitedQueue(objectId)
		}
	}
	if err != nil {
		log.Error("add to file many:", zap.Error(err))
		for _, fb := range req.req.FileBlocks {
			objectId := req.fileIdToObjectId[fb.FileId]
			s.addToRetryUploadingQueue(objectId)
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
		s.updateUploadedCids(objectId, cids)
	}
}
