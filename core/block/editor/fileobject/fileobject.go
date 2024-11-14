package fileobject

import (
	"github.com/anyproto/any-sync/commonfile/fileservice"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
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
	commonFile fileservice.FileService
}

func NewFileObject(sb smartblock.SmartBlock, commonFile fileservice.FileService) FileObject {
	return &fileObject{
		SmartBlock: sb,
		commonFile: commonFile,
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
	return NewFile(f.commonFile, f.getFullFileId(), infos)
}

func (f *fileObject) GetImage() Image {
	infos := filemodels.GetFileInfosFromDetails(f.Details())
	return NewImage(f.commonFile, f.getFullFileId(), infos)
}

func NewFile(commonFile fileservice.FileService, id domain.FullFileId, infos []*storage.FileInfo) File {
	return &file{
		spaceID:    id.SpaceId,
		fileId:     id.FileId,
		info:       infos[0],
		commonFile: commonFile,
	}
}
