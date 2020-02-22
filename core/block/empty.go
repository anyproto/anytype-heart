package block

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var (
	errEmptyBlock = fmt.Errorf("not implemented for this block type")
)

type emptySmart struct {
}

func (e emptySmart) Show() (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Open(b anytype.Block, active bool) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Init() {
	return
}

func (e emptySmart) GetId() (id string) {
	return
}

func (e emptySmart) Active(isActive bool) {
	return
}

func (e emptySmart) Type() (t smartBlockType) {
	return
}

func (e emptySmart) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) CreatePage(req pb.RpcBlockCreatePageRequest) (id, targetId string, err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Duplicate(req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Unlink(id ...string) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Split(id string, pos int32) (blockId string, err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Merge(firstId, secondId string) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Move(req pb.RpcBlockListMoveRequest) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Paste(req pb.RpcBlockPasteRequest) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Replace(id string, block *model.Block) (newId string, err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) UpdateBlock(ids []string, hist bool, apply func(b simple.Block) error) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) UpdateTextBlocks(ids []string, showEvent bool, apply func(t text.Block) error) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) UpdateIconBlock(id string, apply func(t base.IconBlock) error) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Upload(id string, localPath, url string) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) DropFiles(req pb.RpcExternalDropFilesRequest) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) SetFields(fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Undo() (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Redo() (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Close() (err error) {
	err = errEmptyBlock
	return
}

func (e emptySmart) Anytype() anytype.Anytype {
	return nil
}
