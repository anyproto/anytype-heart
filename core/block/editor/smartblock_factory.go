package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/files"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
)

type smartblockFactory struct {
	anytype            core.Service
	fileService        files.IService
	indexer            smartblock.Indexer
	objectStore        objectstore.ObjectStore
	relationService    relation2.Service
	restrictionService restriction.Service
}

func (f smartblockFactory) Produce() smartblock.SmartBlock {
	return smartblock.New(
		f.anytype,
		f.fileService,
		f.restrictionService,
		f.objectStore,
		f.relationService,
		f.indexer,
	)
}
