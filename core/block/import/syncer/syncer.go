package syncer

import "github.com/anytypeio/go-anytype-middleware/core/block/simple"

type Factory struct {
	fs Syncer
	bs Syncer
}

func New(fs *FileSyncer, bs *BookmarkSyncer) *Factory {
	return &Factory{fs:fs, bs: bs}
}

func (f *Factory) GetSyncer(b simple.Block) Syncer {
	if bm := b.Model().GetBookmark(); bm != nil {
		return f.bs
	}
	if file := b.Model().GetFile(); file != nil {
		return f.fs
	}
	return nil
}
