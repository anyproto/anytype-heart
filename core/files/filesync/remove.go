package filesync

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
)

func (s *fileSync) DeleteFile(objectId string, fileId domain.FullFileId) error {
	return fmt.Errorf("TODO")
}

func (s *fileSync) CancelDeletion(objectId string, fileId domain.FullFileId) error {
	return fmt.Errorf("TODO")
}
