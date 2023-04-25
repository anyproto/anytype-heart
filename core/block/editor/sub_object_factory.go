package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/files"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
)

type subObjectFactory struct {
	coreService        core.Service
	fileBlockService   file.BlockService
	fileService        files.Service
	indexer            smartblock.Indexer
	layoutConverter    converter.LayoutConverter
	objectStore        objectstore.ObjectStore
	relationService    relation2.Service
	restrictionService restriction.Service
	sbtProvider        typeprovider.SmartBlockTypeProvider
	sourceService      source.Service
	tempDirProvider    core.TempDirProvider
}

func (f subObjectFactory) produceSmartblock() smartblock.SmartBlock {
	return smartblock.New(
		f.coreService,
		f.fileService,
		f.restrictionService,
		f.objectStore,
		f.relationService,
		f.indexer,
	)
}

func (f subObjectFactory) produce(collection string) (SubObjectImpl, error) {
	sb := f.produceSmartblock()
	switch collection {
	case collectionKeyObjectTypes:
		return NewObjectType(sb, f.objectStore, f.fileBlockService, f.coreService, f.relationService, f.tempDirProvider, f.sbtProvider, f.layoutConverter, f.fileService), nil
	case collectionKeyRelations:
		return NewRelation(sb, f.objectStore, f.fileBlockService, f.coreService, f.relationService, f.tempDirProvider, f.sbtProvider, f.layoutConverter, f.fileService), nil
	case collectionKeyRelationOptions:
		return NewRelationOption(sb, f.objectStore, f.fileBlockService, f.coreService, f.relationService, f.tempDirProvider, f.sbtProvider, f.layoutConverter, f.fileService), nil
	default:
		return nil, fmt.Errorf("unknown collection: %s", collection)
	}
}
