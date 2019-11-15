package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/types"
	mh "github.com/multiformats/go-multihash"
)

type CreateBlockTargetPosition string

const CreateBlockTargetPositionAfter CreateBlockTargetPosition = "after"
const CreateBlockTargetPositionBefore CreateBlockTargetPosition = "before"

func (a *Anytype) GetBlock(id string) (Block, error) {
	_, err := mh.FromB58String(id)
	if err == nil {
		smartBlock, err := a.SmartBlockGet(id)
		if err != nil {
			return nil, err
		}

		switch smartBlock.thread.Schema.Name {
		case "dashboard":
			return &Dashboard{smartBlock}, nil
		case "page":
			return &Page{smartBlock}, nil
		default:
			return nil, fmt.Errorf("for now only smartblocks are queriable")
		}
	}

	// todo: allow to query simple blocks via smart blocks
	return nil, fmt.Errorf("for now only smartblocks are queriable")
}

func isSmartBlock(block *model.Block) bool {
	switch block.Content.(type) {
	case *model.BlockContentOfDashboard, *model.BlockContentOfPage:
		return true
	default:
		return false
	}
}

func (a *Anytype) blockToVersion(block *model.Block, parentSmartBlockVersion BlockVersion, versionId string, user string, date *types.Timestamp) BlockVersion {
	switch block.Content.(type) {
	case *model.BlockContentOfDashboard:
		return &DashboardVersion{&SmartBlockVersion{
			pb: &storage.BlockWithDependentBlocks{
				Block: block,
			},
			versionId: versionId,
			user:      user,
			date:      date,
			node:      a,
		}}
	case *model.BlockContentOfPage:
		return &PageVersion{&SmartBlockVersion{
			pb:        &storage.BlockWithDependentBlocks{Block: block},
			versionId: versionId,
			user:      user,
			date:      date,
			node:      a,
		}}

	default:
		return &SimpleBlockVersion{
			pb:                      block,
			parentSmartBlockVersion: parentSmartBlockVersion,
			node:                    a,
		}
	}
}
