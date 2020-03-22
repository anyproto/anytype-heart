package file

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func NewFile(sb smartblock.SmartBlock, source FileSource) File {
	return &sfile{SmartBlock: sb, fileSource: source}
}

type FileSource interface {
	DoFile(id string, apply func(f File) error) error
}

type File interface {
	DropFiles(req pb.RpcExternalDropFilesRequest) (err error)
	Upload(id string, localPath, url string) (err error)
	UpdateFile(id string, apply func(b file.Block) error) (err error)
}

type sfile struct {
	smartblock.SmartBlock
	fileSource FileSource
}

func (sf *sfile) Upload(id string, localPath, url string) (err error) {
	s := sf.NewState()
	b := s.Get(id)
	f, ok := b.(file.Block)
	if ! ok {
		return fmt.Errorf("not a file block")
	}
	if err = f.Upload(sf.Anytype(), &updater{
		smartId: sf.Id(),
		source:  sf.fileSource,
	}, localPath, url); err != nil {
		return
	}
	return sf.Apply(s)
}

func (sf *sfile) UpdateFile(id string, apply func(b file.Block) error) (err error) {
	s := sf.NewState()
	b := s.Get(id)
	f, ok := b.(file.Block)
	if ! ok {
		return fmt.Errorf("not a file block")
	}
	if err = apply(f); err != nil {
		return
	}
	return sf.Apply(s, smartblock.NoHistory)
}

func (sf *sfile) DropFiles(req pb.RpcExternalDropFilesRequest) (err error) {
	panic("implement me")
}

type updater struct {
	smartId string
	source  FileSource
}

func (u *updater) UpdateFileBlock(id string, apply func(f file.Block)) error {
	return u.source.DoFile(u.smartId, func(f File) error {
		return f.UpdateFile(id, func(b file.Block) error {
			apply(b)
			return nil
		})
	})
}
