package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb"
	"github.com/anytypeio/go-anytype-library/schema"
	mh "github.com/multiformats/go-multihash"
	uuid "github.com/satori/go.uuid"
)

type CreateBlockTargetPosition string

const CreateBlockTargetPositionAfter CreateBlockTargetPosition = "after"
const CreateBlockTargetPositionBefore CreateBlockTargetPosition = "before"

func (a *Anytype) CreateBlock(content pb.IsBlockContent) (Block, error) {
	switch content.(type) {
	case *pb.BlockContentOfPage:
		thrd, err := a.newBlockThread(schema.Page)
		if err != nil {
			return nil, err
		}
		return &Page{SmartBlock{thread: thrd, node: a}}, nil
	case *pb.BlockContentOfDashboard:
		thrd, err := a.newBlockThread(schema.Dashboard)
		if err != nil {
			return nil, err
		}

		return &Dashboard{SmartBlock{thread: thrd, node: a}}, nil
	default:
		return &SimpleBlock{
			id:   uuid.NewV4().String(),
			node: a,
		}, nil
	}
}

func (a *Anytype) AddBlock(target string, targetPosition CreateBlockTargetPosition, content pb.IsBlockContent) (*Block, error) {
	// todo: to be implemented
}

func (a *Anytype) GetBlock(id string) (Block, error) {
	_, err := mh.FromB58String(id)
	if err == nil {
		smartBlock, err := a.SmartBlockGet(id)
		if err != nil {
			return nil, err
		}

		switch smartBlock.thread.Schema.Name {
		case "dashboard":
			return &Dashboard{*smartBlock}, nil
		case "page":
			return &Page{*smartBlock}, nil
		default:
			return nil, fmt.Errorf("for now only smartblocks are queriable")
		}
	}

	// todo: allow to query simple blocks via smart blocks
	return nil, fmt.Errorf("for now only smartblocks are queriable")
}
