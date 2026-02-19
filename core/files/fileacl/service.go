package fileacl

import (
	"fmt"
	"sort"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "fileacl"

type Service interface {
	app.Component

	GetInfoForFileSharing(fileObjectId string) (cid string, encryptionKeys []*model.FileEncryptionKey, err error)
	StoreFileKeys(fileId domain.FileId, fileKeys []*model.FileEncryptionKey) error
}

type service struct {
	fileService       files.Service
	fileObjectService fileobject.Service
	objectStore       objectstore.ObjectStore
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) error {
	s.fileService = app.MustComponent[files.Service](a)
	s.fileObjectService = app.MustComponent[fileobject.Service](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) GetInfoForFileSharing(fileObjectId string) (cid string, encryptionKeys []*model.FileEncryptionKey, err error) {
	var fileId domain.FileId
	if domain.IsFileId(fileObjectId) {
		fileId = domain.FileId(fileObjectId)
	} else {
		fullFileId, err := s.fileObjectService.GetFileIdFromObject(fileObjectId)
		if err != nil {
			return "", nil, fmt.Errorf("get file id from object: %w", err)
		}
		fileId = fullFileId.FileId
	}
	cid = fileId.String()
	keys, err := s.objectStore.GetFileKeys(fileId)
	if err != nil {
		return "", nil, fmt.Errorf("get file keys: %w", err)
	}
	for path, key := range keys {
		encryptionKeys = append(encryptionKeys, &model.FileEncryptionKey{
			Path: path,
			Key:  key,
		})
	}
	sort.Slice(encryptionKeys, func(i, j int) bool {
		return encryptionKeys[i].Path < encryptionKeys[j].Path
	})
	return cid, encryptionKeys, nil
}

func (s *service) StoreFileKeys(fileId domain.FileId, fileKeys []*model.FileEncryptionKey) error {
	if fileId == "" || len(fileKeys) == 0 {
		return nil
	}
	keys := domain.FileEncryptionKeys{
		FileId:         fileId,
		EncryptionKeys: map[string]string{},
	}
	for _, key := range fileKeys {
		keys.EncryptionKeys[key.Path] = key.Key
	}
	err := s.objectStore.AddFileKeys(keys)
	if err != nil {
		return fmt.Errorf("store file encryption keys: %w", err)
	}
	return nil
}
