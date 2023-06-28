package editor

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/typeprovider"
)

type subObjectFactory struct {
	coreService        core.Service
	fileBlockService   file.BlockService
	fileService        files.Service
	indexer            smartblock.Indexer
	layoutConverter    converter.LayoutConverter
	objectStore        objectstore.ObjectStore
	relationService    relation.Service
	restrictionService restriction.Service
	sbtProvider        typeprovider.SmartBlockTypeProvider
	sourceService      source.Service
	tempDirProvider    core.TempDirProvider
	picker             getblock.Picker
	eventSender        event.Sender
}

func (f subObjectFactory) produceSmartblock() smartblock.SmartBlock {
	return smartblock.New(
		f.coreService,
		f.fileService,
		f.restrictionService,
		f.objectStore,
		f.relationService,
		f.indexer,
		f.eventSender,
	)
}

func (f subObjectFactory) produce(collection string) (SubObjectImpl, error) {
	sb := f.produceSmartblock()
	switch collection {
	case collectionKeyObjectTypes:
		return NewObjectType(sb, f.objectStore, f.fileBlockService, f.coreService, f.relationService, f.tempDirProvider, f.sbtProvider, f.layoutConverter, f.fileService, f.picker, f.eventSender), nil
	case collectionKeyRelations:
		return NewRelation(sb, f.objectStore, f.fileBlockService, f.coreService, f.relationService, f.tempDirProvider, f.sbtProvider, f.layoutConverter, f.fileService, f.picker, f.eventSender), nil
	case collectionKeyRelationOptions:
		return NewRelationOption(sb, f.objectStore, f.fileBlockService, f.coreService, f.relationService, f.tempDirProvider, f.sbtProvider, f.layoutConverter, f.fileService, f.picker, f.eventSender), nil
	default:
		return nil, fmt.Errorf("unknown collection: %s", collection)
	}
}
