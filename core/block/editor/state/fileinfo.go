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

func (s *State) GetFileInfo() FileInfo {
	return s.fileInfo
}

func (s *State) SetFileInfo(info FileInfo) {
	s.fileInfo = info
}

func (s *State) diffFileInfo() []*pb.ChangeContent {
	if s.parent != nil && !s.parent.fileInfo.Equals(s.fileInfo) {
		keys := make([]*model.FileInfoEncryptionKey, 0, len(s.fileInfo.EncryptionKeys))
		for path, key := range s.fileInfo.EncryptionKeys {
			keys = append(keys, &model.FileInfoEncryptionKey{
				Path: path,
				Key:  key,
			})
		}
		return []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfSetFileInfo{
					SetFileInfo: &pb.ChangeSetFileInfo{
						FileInfo: &model.FileInfo{
							Hash:           s.fileInfo.Hash,
							EncryptionKeys: keys,
						},
					},
				},
			},
		}
	}
	return nil
}

func (s *State) changeSetFileInfo(ch *pb.ChangeSetFileInfo) {
	keys := make(map[string]string, len(ch.FileInfo.EncryptionKeys))
	for _, key := range ch.FileInfo.EncryptionKeys {
		keys[key.Path] = key.Key
	}
	s.SetFileInfo(FileInfo{
		Hash:           ch.FileInfo.Hash,
		EncryptionKeys: keys,
	})
}
