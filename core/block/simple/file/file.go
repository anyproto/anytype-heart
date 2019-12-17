package file

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/mohae/deepcopy"
)

func init() {
	simple.RegisterCreator(NewFile)
}

func NewFile(m *model.Block) simple.Block {
	if file := m.GetFile(); file != nil {
		if file.State == model.BlockContentFile_Uploading {
			if file.LocalFilePath != "" {
				file.State = model.BlockContentFile_Done
			} else {
				file.State = model.BlockContentFile_Error
			}
		}
		return &File{
			Base:    base.NewBase(m).(*base.Base),
			content: file,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	Upload(localPath, url string) error
}

type File struct {
	*base.Base
	content *model.BlockContentFile
}

func (f *File) Upload(localPath, url string) (err error) {
	if f.content.State != model.BlockContentFile_Empty && f.content.State != model.BlockContentFile_Error {
		return fmt.Errorf("block is not empty")
	}
	f.content.State = model.BlockContentFile_Uploading
	return
}

func (f *File) Copy() simple.Block {
	return NewFile(deepcopy.Copy(f.Model()).(*model.Block))
}

func (f *File) setFile(cf *core.File) {

}
