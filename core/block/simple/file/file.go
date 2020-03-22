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
		return &File{
			Base:    base.NewBase(m).(*base.Base),
			content: file,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	Upload(stor anytype.Service, updater Updater, localPath, url string) (err error)
	SetFileData(hash string, meta core.FileMeta)
	SetImage(hash, name string)
	SetState(state model.BlockContentFileState)
}

type Updater interface {
	UpdateFileBlock(id string, apply func(f Block)) error
}

type File struct {
	*base.Base
	content *model.BlockContentFile
}

func (f *File) Upload(stor anytype.Service, updater Updater, localPath, url string) (err error) {
	if f.content.State != model.BlockContentFile_Empty && f.content.State != model.BlockContentFile_Error {
		return fmt.Errorf("block is not empty")
	}
	f.content.State = model.BlockContentFile_Uploading
	id := f.Id
	up := &uploader{
		updateFile: func(apply func(file Block)) {
			if e := updater.UpdateFileBlock(id, apply); e != nil {
				log.Warnf("can't update file block: %v", e)
			}
		},
		storage: stor,
	}
	if f.content.Type == model.BlockContentFile_Image {
		go up.DoImage(localPath, url)
	} else {
		go up.Do(localPath, url)
	}
	return
}

func (f *File) Copy() simple.Block {
	copy := deepcopy.Copy(f.Model()).(*model.Block)
	return &File{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetFile(),
	}
}

func (f *File) SetState(state model.BlockContentFileState) {
	f.content.State = state
}

func (f *File) SetFileData(hash string, meta core.FileMeta) {
	f.content.Size_ = meta.Size
	if strings.HasPrefix(meta.Media, "video/") {
		f.content.Type = model.BlockContentFile_Video
	} else {
		f.content.Type = model.BlockContentFile_File
	}
	f.content.State = model.BlockContentFile_Done
	f.content.Name = meta.Name
	f.content.Hash = hash
	f.content.Mime = meta.Media
}

func (f *File) SetImage(hash, name string) {
	f.content.Type = model.BlockContentFile_Image
	f.content.State = model.BlockContentFile_Done
	f.content.Name = name
	f.content.Hash = hash
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
	if f.content.Hash != file.content.Hash {
		hasChanges = true
		changes.Hash = &pb.EventBlockSetFileHash{Value: file.content.Hash}
	}
	if f.content.Name != file.content.Name {
		hasChanges = true
		changes.Name = &pb.EventBlockSetFileName{Value: file.content.Name}
	}
	if f.content.Size_ != file.content.Size_ {
		hasChanges = true
		changes.Size_ = &pb.EventBlockSetFileSize{Value: file.content.Size_}
	}
	if f.content.Mime != file.content.Mime {
		hasChanges = true
		changes.Mime = &pb.EventBlockSetFileMime{Value: file.content.Mime}
	}

	if hasChanges {
		msgs = append(msgs, &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetFile{BlockSetFile: changes}})
	}
	return
}
