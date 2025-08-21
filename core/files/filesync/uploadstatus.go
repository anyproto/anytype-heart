package filesync

import (
	"sync"

	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
)

type uploadStatusIndex struct {
	lock  sync.Mutex
	files map[string]map[cid.Cid]struct{}

	fileIdToSpaceId      map[string]string
	fileIdToFileObjectId map[string]string
}

type fileBlocksIndex struct {
	fileId         string
	fileObjectId   string
	blocksToUpload map[cid.Cid]struct{}
}

func newUploadStatusIndex() *uploadStatusIndex {
	return &uploadStatusIndex{
		files:                make(map[string]map[cid.Cid]struct{}),
		fileIdToSpaceId:      make(map[string]string),
		fileIdToFileObjectId: make(map[string]string),
	}
}

func (i *uploadStatusIndex) add(fileObjectId string, spaceId string, fileId string, c cid.Cid) {
	i.lock.Lock()
	defer i.lock.Unlock()
	cidsPerFile := i.files[fileId]
	if cidsPerFile == nil {
		cidsPerFile = map[cid.Cid]struct{}{}
		i.files[fileId] = cidsPerFile
	}
	cidsPerFile[c] = struct{}{}

	i.fileIdToSpaceId[fileId] = spaceId
	i.fileIdToFileObjectId[fileId] = fileObjectId
}

func (i *uploadStatusIndex) remove(fileId string, c cid.Cid, onUploaded func(objectId string, fullFileId domain.FullFileId) error) {
	i.lock.Lock()
	cidsPerFile := i.files[fileId]
	delete(cidsPerFile, c)
	objectId := i.fileIdToFileObjectId[fileId]
	spaceId := i.fileIdToSpaceId[fileId]
	if len(cidsPerFile) == 0 {
		i.lock.Unlock()
		err := onUploaded(objectId, domain.FullFileId{SpaceId: spaceId, FileId: domain.FileId(fileId)})
		if err != nil {
			log.Error("on uploaded callback", zap.Error(err))
		}
		i.lock.Lock()
		delete(i.files, fileId)
		i.lock.Unlock()
	} else {
		i.lock.Unlock()
	}
}
