package file

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
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
	Upload(stor anytype.Anytype, updater Updater, localPath, url string) (err error)
	SetFile(cf *core.File)
	SetState(state model.BlockContentFileState)
}

type Updater interface {
	UpdateFileBlock(id string, apply func(f Block)) error
}

type File struct {
	*base.Base
	content *model.BlockContentFile
}

func (f *File) Upload(stor anytype.Anytype, updater Updater, localPath, url string) (err error) {
	if f.content.State != model.BlockContentFile_Empty && f.content.State != model.BlockContentFile_Error {
		return fmt.Errorf("block is not empty")
	}
	f.content.State = model.BlockContentFile_Uploading
	id := f.Id
	up := &uploader{
		updateFile: func(apply func(file Block)) {
			if e := updater.UpdateFileBlock(id, apply); e != nil {
				fmt.Println("can't update file block:", e)
			}
		},
		storage: stor,
	}
	go up.Do(localPath, url)
	return
}

func (f *File) Copy() simple.Block {
	return NewFile(deepcopy.Copy(f.Model()).(*model.Block))
}

func (f *File) SetState(state model.BlockContentFileState) {
	f.content.State = state
}

func (f *File) SetFile(cf *core.File) {
	meta := cf.Meta()
	f.content.Size_ = meta.Size
	if strings.HasPrefix(meta.Media, "image/") {
		f.content.Type = model.BlockContentFile_Image
	} else if strings.HasPrefix(meta.Media, "video/") {
		f.content.Type = model.BlockContentFile_Video
	}
	f.content.State = model.BlockContentFile_Done
	// TODO: set name
}

func (f *File) Diff(b simple.Block) (msgs []*pb.EventMessage, err error) {
	file, ok := b.(*File)
	if ! ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = f.Base.Diff(file); err != nil {
		return
	}
	changes := &pb.EventBlockSetFile{
		Id: file.Id,
	}
	hasChanges := false

	if f.content.State != file.content.State {
		hasChanges = true
		changes.State = &pb.EventBlockSetFileState{Value: file.content.State}
	}
	if f.content.Type != file.content.Type {
		hasChanges = true
		changes.Type = &pb.EventBlockSetFileType{Value: file.content.Type}
	}
	if f.content.LocalFilePath != file.content.LocalFilePath {
		hasChanges = true
		changes.LocalFilePath = &pb.EventBlockSetFileLocalFilePath{Value: file.content.LocalFilePath}
	}
	if f.content.PreviewFilePath != file.content.PreviewFilePath {
		hasChanges = true
		changes.PreviewLocalFilePath = &pb.EventBlockSetFilePreviewLocalFilePath{Value: file.content.PreviewFilePath}
	}
	
	if hasChanges {
		msgs = append(msgs, &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetFile{BlockSetFile: changes}})
	}
	return
}
