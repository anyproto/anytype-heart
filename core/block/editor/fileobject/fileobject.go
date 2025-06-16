package fileobject

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-editor-fileobject")

type FileObject interface {
	GetFile() (files.File, error)
	GetImage() (files.Image, error)
	GetFullFileId() domain.FullFileId
}

type fileObject struct {
	smartblock.SmartBlock
	fileService files.Service
}

func NewFileObject(sb smartblock.SmartBlock, fileService files.Service) FileObject {
	return &fileObject{
		SmartBlock:  sb,
		fileService: fileService,
	}
}

func (f *fileObject) GetFullFileId() domain.FullFileId {
	return domain.FullFileId{
		SpaceId: f.SpaceID(),
		FileId:  domain.FileId(f.Details().GetString(bundle.RelationKeyFileId)),
	}
}

func (f *fileObject) GetFile() (files.File, error) {
	infos, err := filemodels.GetFileInfosFromDetails(f.Details())
	if err != nil {
		return nil, fmt.Errorf("get file infos from details: %w", err)
	}
	return files.NewFile(f.fileService, f.GetFullFileId(), infos)
}

func (f *fileObject) GetImage() (files.Image, error) {
	infos, err := filemodels.GetFileInfosFromDetails(f.Details())
	if err != nil {
		return nil, fmt.Errorf("get file infos from details: %w", err)
	}
	return files.NewImage(f.fileService, f.GetFullFileId(), infos), nil
}
