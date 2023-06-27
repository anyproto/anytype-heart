package editor

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/clipboard"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/typeprovider"
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
	relationService relation.Service,
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
	fileService files.Service,
	picker getblock.Picker,
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
				picker,
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
