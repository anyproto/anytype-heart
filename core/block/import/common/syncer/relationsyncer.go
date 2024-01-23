package syncer

import (
	"context"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type RelationSyncer interface {
	Sync(spaceID string, snapshotPayloads map[string]treestorage.TreeStorageCreatePayload, state *state.State, relationName string, origin model.ObjectOrigin) []string
}

type FileRelationSyncer struct {
	service           *block.Service
	objectStore       objectstore.ObjectStore
	fileStore         filestore.FileStore
	fileObjectService fileobject.Service
}

func NewFileRelationSyncer(service *block.Service, fileStore filestore.FileStore, fileObjectService fileobject.Service, objectStore objectstore.ObjectStore) RelationSyncer {
	return &FileRelationSyncer{
		service:           service,
		fileStore:         fileStore,
		fileObjectService: fileObjectService,
		objectStore:       objectStore,
	}
}

func (fs *FileRelationSyncer) Sync(spaceID string, snapshotPayloads map[string]treestorage.TreeStorageCreatePayload, st *state.State, relationName string, origin model.ObjectOrigin) []string {
	fileIds := fs.getFilesFromRelations(st, relationName)
	var allFilesHashes, filesToDelete []string
	for _, fileId := range fileIds {
		if fileId == "" {
			continue
		}
		fileObjectId := fs.uploadFile(spaceID, snapshotPayloads, fileId, origin)
		if fileObjectId != "" {
			allFilesHashes = append(allFilesHashes, fileObjectId)
			filesToDelete = append(filesToDelete, fileObjectId)
		}
	}
	fs.updateFileRelationsDetails(st, relationName, allFilesHashes)
	return filesToDelete
}

func (fs *FileRelationSyncer) getFilesFromRelations(st *state.State, name string) []string {
	var allFiles []string
	if files := pbtypes.GetStringList(st.Details(), name); len(files) > 0 {
		allFiles = append(allFiles, files...)
	}

	if files := pbtypes.GetString(st.Details(), name); files != "" {
		allFiles = append(allFiles, files)
	}
	return allFiles
}

func (fs *FileRelationSyncer) uploadFile(spaceID string, snapshotPayloads map[string]treestorage.TreeStorageCreatePayload, file string, origin model.ObjectOrigin) string {
	var (
		fileObjectId string
		err          error
	)
	if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") {
		req := block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{Url: file},
			Origin:               origin,
		}
		fileObjectId, _, err = fs.service.UploadFile(context.Background(), spaceID, req)
		if err != nil {
			log.Errorf("file uploading %s", err)
		}
	} else {
		if _, ok := snapshotPayloads[file]; ok {
			return file
		}
		_, err = cid.Decode(file)
		if err == nil {
			fileObjectId, err = fs.fileObjectService.CreateFromImport(domain.FullFileId{SpaceId: spaceID, FileId: domain.FileId(file)}, origin)
			if err != nil {
				log.With("fileId", file).Errorf("create file object: %v", err)
				return file
			}
			return fileObjectId
		}
		req := block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{LocalPath: file},
			Origin:               origin,
		}
		fileObjectId, _, err = fs.service.UploadFile(context.Background(), spaceID, req)
		if err != nil {
			log.Errorf("file uploading %s", err)
		}
	}
	return fileObjectId
}

func (fs *FileRelationSyncer) updateFileRelationsDetails(st *state.State, name string, allFilesHashes []string) {
	if st.Details() == nil || st.Details().GetFields() == nil {
		return
	}
	if st.Details().Fields[name].GetListValue() != nil {
		st.SetDetail(name, pbtypes.StringList(allFilesHashes))
	}
	hash := ""
	if len(allFilesHashes) > 0 {
		hash = allFilesHashes[0]
	}
	if st.Details().Fields[name].GetStringValue() != "" {
		st.SetDetail(name, pbtypes.String(hash))
	}
}
