package filemodels

import (
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
)

type CreateRequest struct {
	FileId                domain.FileId
	EncryptionKeys        map[string]string
	ObjectOrigin          objectorigin.ObjectOrigin
	AdditionalDetails     *types.Struct
	AsyncMetadataIndexing bool
}

var (
	ErrObjectNotFound = fmt.Errorf("file object not found")
	ErrEmptyFileId    = fmt.Errorf("empty file id")
)
