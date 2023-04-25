package syncer

import "github.com/anytypeio/go-anytype-middleware/core/block/simple"

type Factory struct {
	fs Syncer
	is Syncer
}

func New(fs *FileSyncer, is *IconSyncer) *Factory {
	return &Factory{fs: fs, is: is}
}

func (f *Factory) GetSyncer(b simple.Block) Syncer {
	if file := b.Model().GetFile(); file != nil {
		return f.fs
	}
	if b.Model().GetText() != nil && b.Model().GetText().GetIconImage() != "" {
		return f.is
	}
	return nil
}
