package core

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
)

type Dashboard struct {
	*SmartBlock
}

// NewBlock should be used as constructor for the new block
func (dashboard *Dashboard) NewBlock(block model.Block) (Block, error) {
	return dashboard.newBlock(block, dashboard)
}
