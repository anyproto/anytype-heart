package core

import (
	"context"
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"
)

type BlockVersionMeta interface {
	VersionId() string
	Model() *model.BlockMetaOnly
	User() string
	Date() *types.Timestamp
	// ExternalFields returns fields supposed to be viewable when block not opened
	ExternalFields() *types.Struct
}

var ErrorNotSmartBlock = fmt.Errorf("can't retrieve thread for not smart block")

func (a *Anytype) getThreadForBlock(b *model.Block) (thread.Info, error) {
	switch b.Content.(type) {
	case *model.BlockContentOfPage, *model.BlockContentOfDashboard:
		tid, err := thread.Decode(b.Id)
		if err != nil {
			return thread.Info{}, err
		}
		thrd, err := a.ts.GetThread(context.TODO(), tid)
		if err != nil {
			return thread.Info{}, err
		}

		return thrd, nil
	default:
		return thread.Info{}, ErrorNotSmartBlock
	}
}

func blockRestrictionsEmpty() model.BlockRestrictions {
	return model.BlockRestrictions{
		Read:   false,
		Edit:   false,
		Remove: false,
		Drag:   false,
		DropOn: false,
	}
}
