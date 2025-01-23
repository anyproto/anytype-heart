package file

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	simple.FileHashes
	TargetObjectId() string
	SetHash(hash string) Block
	SetName(name string) Block
	SetState(state model.BlockContentFileState) Block
	SetType(tp model.BlockContentFileType) Block
	SetStyle(tp model.BlockContentFileStyle) Block
	SetSize(size int64) Block
	SetMIME(mime string) Block
	SetTime(tm time.Time) Block
	SetModel(m *model.BlockContentFile) Block
	SetTargetObjectId(value string) Block
	ApplyEvent(e *pb.EventBlockSetFile) error
}

type Updater interface {
	UpdateFileBlock(id string, apply func(f Block)) error
}

type File struct {
	*base.Base
	content *model.BlockContentFile
}

func (f *File) TargetObjectId() string {
	return f.content.TargetObjectId
}

func (f *File) SetHash(hash string) Block {
	f.content.Hash = hash
	return f
}

func (f *File) SetTargetObjectId(value string) Block {
	f.content.TargetObjectId = value
	return f
}

func (f *File) SetName(name string) Block {
	f.content.Name = name
	return f
}

func (f *File) SetState(state model.BlockContentFileState) Block {
	f.content.State = state
	return f
}

func (f *File) SetType(tp model.BlockContentFileType) Block {
	f.content.Type = tp
	return f
}

func (f *File) SetStyle(tp model.BlockContentFileStyle) Block {
	f.content.Style = tp
	return f
}

func (f *File) SetSize(size int64) Block {
	f.content.Size_ = size
	return f
}

func (f *File) SetMIME(mime string) Block {
	f.content.Mime = mime
	return f
}

func (f *File) SetTime(tm time.Time) Block {
	f.content.AddedAt = tm.Unix()
	return f
}

func (f *File) SetModel(m *model.BlockContentFile) Block {
	f.content.Hash = m.Hash
	f.content.Type = m.Type
	f.content.Name = m.Name
	f.content.AddedAt = m.AddedAt
	f.content.Mime = m.Mime
	f.content.Style = m.Style
	f.content.Size_ = m.Size_
	f.content.State = m.State
	f.content.TargetObjectId = m.TargetObjectId
	return f
}

func (f *File) Copy() simple.Block {
	copy := pbtypes.CopyBlock(f.Model())
	return &File{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetFile(),
	}
}

func (f *File) Validate() error {
	return nil
}

func (f *File) Diff(spaceId string, b simple.Block) (msgs []simple.EventMessage, err error) {
	file, ok := b.(*File)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = f.Base.Diff(spaceId, file); err != nil {
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
	if f.content.Style != file.content.Style {
		hasChanges = true
		changes.Style = &pb.EventBlockSetFileStyle{Value: file.content.Style}
	}
	if f.content.TargetObjectId != file.content.TargetObjectId {
		hasChanges = true
		changes.TargetObjectId = &pb.EventBlockSetFileTargetObjectId{Value: file.content.TargetObjectId}
	}

	if hasChanges {
		msgs = append(msgs, simple.EventMessage{Msg: event.NewMessage(spaceId, &pb.EventMessageValueOfBlockSetFile{BlockSetFile: changes})})
	}
	return
}

func (f *File) ApplyEvent(e *pb.EventBlockSetFile) error {
	if e.Type != nil {
		f.content.Type = e.Type.GetValue()
		if f.content.Type == model.BlockContentFile_File {
			name := f.content.Name
			if e.Name != nil {
				name = e.Name.GetValue()
			}
			f.content.Type = DetectTypeByMIME(name, f.content.GetMime())
		}
	}
	if e.State != nil {
		f.content.State = e.State.GetValue()
	}
	if e.Hash != nil {
		f.content.Hash = e.Hash.GetValue()
	}
	if e.Name != nil {
		f.content.Name = e.Name.GetValue()
	}
	if e.Mime != nil {
		f.content.Mime = e.Mime.GetValue()
	}
	if e.Style != nil {
		f.content.Style = e.Style.GetValue()
	}
	if e.Size_ != nil {
		f.content.Size_ = e.Size_.GetValue()
	}
	if e.TargetObjectId != nil {
		f.content.TargetObjectId = e.TargetObjectId.GetValue()
	}
	return nil
}

func (f *File) IterateLinkedFiles(iter func(id string)) {
	if f.content.TargetObjectId != "" {
		iter(f.content.TargetObjectId)
	}
}

func (f *File) FillFileHashes(hashes []string) []string {
	if f.content.Hash != "" {
		return append(hashes, f.content.Hash)
	}
	return hashes
}

func (f *File) MigrateFile(migrateFunc func(oldHash string) (newHash string)) {
	if f.content.TargetObjectId != "" {
		return
	}
	f.content.TargetObjectId = migrateFunc(f.content.Hash)
}

func (f *File) FillSmartIds(ids []string) []string {
	if f.content.TargetObjectId != "" {
		return append(ids, f.content.TargetObjectId)
	}
	return ids
}

func (f *File) HasSmartIds() bool {
	return f.content.TargetObjectId != ""
}

func (f *File) IsEmpty() bool {
	return f.content.TargetObjectId == "" && f.content.Hash == ""
}

func DetectTypeByMIME(name, mime string) model.BlockContentFileType {
	if filepath.Ext(name) == constant.SvgExt {
		return model.BlockContentFile_Image
	}
	if mill.IsImage(mime) {
		return model.BlockContentFile_Image
	}
	if strings.HasPrefix(mime, "video") {
		return model.BlockContentFile_Video
	}
	if strings.HasPrefix(mime, "audio") {
		return model.BlockContentFile_Audio
	}
	if strings.HasPrefix(mime, "application/pdf") {
		return model.BlockContentFile_PDF
	}

	return model.BlockContentFile_File
}
