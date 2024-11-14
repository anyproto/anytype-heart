package fileobject

import (
	"github.com/anyproto/any-sync/commonfile/fileservice"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/files"
)

type FileObject interface {
	GetFile() files.File
	GetImage() files.Image
}

type fileObject struct {
	smartblock.SmartBlock
	commonFile fileservice.FileService
}

func NewFileObject(sb smartblock.SmartBlock, commonFile fileservice.FileService) FileObject {
	return &fileObject{
		SmartBlock: sb,
		commonFile: commonFile,
	}
}

func (f *fileObject) GetFile() files.File {
	return nil
}

func (f *fileObject) GetImage() files.Image {
	return nil
}
