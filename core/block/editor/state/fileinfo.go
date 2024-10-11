package state

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type FileInfo struct {
	FileId         domain.FileId
	EncryptionKeys map[string]string
}

func (f FileInfo) Equals(other FileInfo) bool {
	if f.FileId != other.FileId {
		return false
	}
	if len(f.EncryptionKeys) != len(other.EncryptionKeys) {
		return false
	}
	for k, v := range f.EncryptionKeys {
		if other.EncryptionKeys[k] != v {
			return false
		}
	}
	return true
}

func (f FileInfo) ToModel() *model.FileInfo {
	if f.FileId == "" {
		return nil
	}
	keys := make([]*model.FileEncryptionKey, 0, len(f.EncryptionKeys))
	for path, key := range f.EncryptionKeys {
		keys = append(keys, &model.FileEncryptionKey{
			Path: path,
			Key:  key,
		})
	}
	return &model.FileInfo{
		FileId:         f.FileId.String(),
		EncryptionKeys: keys,
	}
}

func (s *State) GetFileInfo() FileInfo {
	return s.fileInfo
}

func (s *State) SetFileInfo(info FileInfo) {
	s.fileInfo = info
}

func (s *State) diffFileInfo() []*pb.ChangeContent {
	if s.parent != nil && !s.parent.fileInfo.Equals(s.fileInfo) {
		return []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfSetFileInfo{
					SetFileInfo: &pb.ChangeSetFileInfo{
						FileInfo: s.fileInfo.ToModel(),
					},
				},
			},
		}
	}
	return nil
}

func (s *State) setFileInfoFromModel(fileInfo *model.FileInfo) {
	if fileInfo == nil {
		return
	}
	keys := make(map[string]string, len(fileInfo.EncryptionKeys))
	for _, key := range fileInfo.EncryptionKeys {
		keys[key.Path] = key.Key
	}
	s.SetFileInfo(FileInfo{
		FileId:         domain.FileId(fileInfo.FileId),
		EncryptionKeys: keys,
	})
}
