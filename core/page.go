package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
)

const (
	defaultDocName = "Untitled"
)

var errorNotFound = fmt.Errorf("not found")

type Page struct {
	*SmartBlock
}

// NewBlock should be used as constructor for the new block
func (page *Page) NewBlock(block model.Block) (Block, error) {
	return page.newBlock(block, page)
}
