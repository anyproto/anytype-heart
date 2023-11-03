package syncer

import (
	"context"
	"strings"

	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type RelationSyncer interface {
	Sync(spaceID string, state *state.State, relationName string, origin model.ObjectOrigin) []string
}

type FileRelationSyncer struct {
	service   *block.Service
	fileStore filestore.FileStore
}

func NewFileRelationSyncer(service *block.Service, fileStore filestore.FileStore) RelationSyncer {
	return &FileRelationSyncer{service: service, fileStore: fileStore}
}

func (fs *FileRelationSyncer) Sync(spaceID string, state *state.State, relationName string, origin model.ObjectOrigin) []string {
	allFiles := fs.getFilesFromRelations(state, relationName)
	var allFilesHashes, filesToDelete []string
	for _, f := range allFiles {
		if f == "" {
			continue
		}
		var hash string
		if hash = fs.uploadFile(spaceID, f, origin); hash != "" {
			allFilesHashes = append(allFilesHashes, hash)
			filesToDelete = append(filesToDelete, hash)
		}
		if hash == "" {
			if targets, err := fs.fileStore.ListByTarget(f); err == nil && len(targets) > 0 {
				allFilesHashes = append(allFilesHashes, f)
				continue
			}
		}
	}
	fs.updateFileRelationsDetails(state, relationName, allFilesHashes)
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

func (fs *FileRelationSyncer) uploadFile(spaceID string, file string, origin model.ObjectOrigin) string {
	var (
		hash string
		err  error
	)
	if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") {
		req := block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{Url: file},
			Origin:               origin,
		}
		hash, err = fs.service.UploadFile(context.Background(), spaceID, req)
		if err != nil {
			log.Errorf("file uploading %s", err)
		}
	} else {
		_, err = cid.Decode(file)
		if err == nil {
			return file
		}
		req := block.FileUploadRequest{
			RpcFileUploadRequest: pb.RpcFileUploadRequest{LocalPath: file},
			Origin:               origin,
		}
		hash, err = fs.service.UploadFile(context.Background(), spaceID, req)
		if err != nil {
			log.Errorf("file uploading %s", err)
		}
	}
	return hash
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
