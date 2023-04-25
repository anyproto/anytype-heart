package editor

import (
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/clipboard"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/files"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
)

type SubObject struct {
	smartblock.SmartBlock

	basic.AllOperations
	basic.IHistory
	stext.Text
	clipboard.Clipboard
	dataview.Dataview
}

func NewSubObject(
	sb smartblock.SmartBlock,
	objectStore objectstore.ObjectStore,
	fileBlockService file.BlockService,
	anytype core.Service,
	relationService relation2.Service,
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
	fileService files.IService,
) *SubObject {
	return &SubObject{
		SmartBlock:    sb,
		AllOperations: basic.NewBasic(sb, objectStore, relationService, layoutConverter),
		IHistory:      basic.NewHistory(sb),
		Text: stext.NewText(
			sb,
			objectStore,
		),
		Clipboard: clipboard.NewClipboard(
			sb,
			file.NewFile(
				sb,
				fileBlockService,
				tempDirProvider,
				fileService,
			),
			tempDirProvider,
			relationService,
			fileService,
		),
		Dataview: dataview.NewDataview(
			sb,
			anytype,
			objectStore,
			relationService,
			sbtProvider,
		),
	}
}

func (o *SubObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = o.SmartBlock.Init(ctx); err != nil {
		return
	}

	return nil
}

func (o *SubObject) SetStruct(st *types.Struct) error {
	o.Lock()
	defer o.Unlock()
	s := o.NewState()
	s.SetDetails(st)
	return o.Apply(s)
}
