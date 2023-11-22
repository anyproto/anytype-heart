package state

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type FileInfo struct {
	FileHash      string
	EncryptionKey string
}

func (s *State) GetFileInfo() FileInfo {
	return s.fileInfo
}

func (s *State) SetFileInfo(info FileInfo) {
	s.fileInfo = info
}

func (s *State) diffFileInfo() []*pb.ChangeContent {
	if s.parent != nil && s.parent.fileInfo != s.fileInfo {
		return []*pb.ChangeContent{
			{
				Value: &pb.ChangeContentValueOfSetFileInfo{
					SetFileInfo: &pb.ChangeSetFileInfo{
						FileInfo: &model.FileInfo{
							FileHash:      s.fileInfo.FileHash,
							EncryptionKey: s.fileInfo.EncryptionKey,
						},
					},
				},
			},
		}
	}
	return nil
}

func (s *State) changeSetFileInfo(ch *pb.ChangeSetFileInfo) {
	s.SetFileInfo(FileInfo{
		FileHash:      ch.FileInfo.FileHash,
		EncryptionKey: ch.FileInfo.EncryptionKey,
	})
}
