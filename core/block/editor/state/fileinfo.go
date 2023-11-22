package state

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type FileInfo struct {
	Hash           string
	EncryptionKeys map[string]string
}

func (f FileInfo) Equals(other FileInfo) bool {
	if f.Hash != other.Hash {
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
	if f.Hash == "" {
		return nil
	}
	keys := make([]*model.FileInfoEncryptionKey, 0, len(f.EncryptionKeys))
	for path, key := range f.EncryptionKeys {
		keys = append(keys, &model.FileInfoEncryptionKey{
			Path: path,
			Key:  key,
		})
	}
	return &model.FileInfo{
		Hash:           f.Hash,
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
		Hash:           fileInfo.Hash,
		EncryptionKeys: keys,
	})
}
