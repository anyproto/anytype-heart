package _import

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type Import interface {
	ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinkIds []string, err error)
}

func NewImport(sb smartblock.SmartBlock) Import {
	return &importImpl{sb}
}

type importImpl struct {
	smartblock.SmartBlock
}

func (imp *importImpl) ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinkIds []string, err error) {
	// ...
	return rootLinkIds, err
}
