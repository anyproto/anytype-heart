package core

import (
	"fmt"

	mh "github.com/multiformats/go-multihash"
	uuid "github.com/satori/go.uuid"
)

func (a *Anytype) CreateBlock(blockType BlockType) (*Block, error) {
	switch blockType {
	case BlockType_PAGE, BlockType_DASHBOARD, BlockType_DATAVIEW:
		smartBlock, err := a.SmartBlockCreate(blockType)
		if err != nil {
			return nil, err
		}

		return &Block{Id: smartBlock.GetId(), Type: blockType}, nil
	default:
		return &Block{
			Id:   uuid.NewV4().String(),
			Type: blockType,
		}, nil
	}
}

func (a *Anytype) GetBlock(id string) (*Block, error) {
	_, err := mh.FromB58String(id)
	if err == nil {
		smartBlock, err := a.SmartBlockGet(id)
		if err != nil {
			return nil, err
		}

		return &Block{Id: id, Type: smartBlock.GetType()}, nil
	}

	// todo: allow to query simple blocks via smart blocks
	return nil, fmt.Errorf("for now only smartblocks are queriable")
}
