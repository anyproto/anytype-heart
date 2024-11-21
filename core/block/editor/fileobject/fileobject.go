package fileobject

import (
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("anytype-mw-editor-fileobject")

type FileObject interface {
	GetFile() File
	GetImage() Image
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

func (f *fileObject) getFullFileId() domain.FullFileId {
	return domain.FullFileId{
		SpaceId: f.SpaceID(),
		FileId:  domain.FileId(pbtypes.GetString(f.LocalDetails(), bundle.RelationKeyFileId.String())),
	}
}

func (f *fileObject) GetFile() File {
	infos := filemodels.GetFileInfosFromDetails(f.Details())
	return NewFile(f.fileService, f.getFullFileId(), infos)
}

func (f *fileObject) GetImage() Image {
	infos := filemodels.GetFileInfosFromDetails(f.Details())
	return NewImage(f.fileService, f.getFullFileId(), infos)
}

func NewFile(fileService files.Service, id domain.FullFileId, infos []*storage.FileInfo) File {
	return &file{
		spaceID:     id.SpaceId,
		fileId:      id.FileId,
		info:        infos[0],
		fileService: fileService,
	}
}
