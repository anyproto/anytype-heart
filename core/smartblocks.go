package core

import (
	"context"
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/pb/storage"
	"github.com/gogo/protobuf/types"

	"github.com/textileio/go-threads/core/thread"
)

var ErrorNoBlockVersionsFound = fmt.Errorf("no block versions found")

func (a *Anytype) newBlockThread(blockType SmartBlockType) (thread.Info, error) {
	thrdId, err := newThreadID(thread.AccessControlled, blockType)
	if err != nil {
		return thread.Info{}, err
	}
	return a.ts.CreateThread(context.TODO(), thrdId)
}

func (a *Anytype) GetSmartBlock(id string) (*SmartBlock, error) {
	thrd, _ := a.predefinedThreadByName(id)
	if thrd.ID == thread.Undef {
		tid, err := thread.Decode(id)

		if err != nil {
			return nil, err
		}

		thrd, err = a.ts.GetThread(context.TODO(), tid)
		if err != nil {
			return nil, err
		}
	}

	return &SmartBlock{thread: thrd, node: a}, nil
}

func (a *Anytype) smartBlockVersionWithFullRestrictions(id string) *SmartBlockVersion {
	return &SmartBlockVersion{
		node: a,
		model: &storage.BlockWithMeta{
			Block: &model.Block{
				Id: id,
				Fields: &types.Struct{Fields: map[string]*types.Value{
					"name": {Kind: &types.Value_StringValue{StringValue: "Inaccessible block"}},
					"icon": {Kind: &types.Value_StringValue{StringValue: ":no_entry_sign:"}},
				}},
				Restrictions: &model.BlockRestrictions{
					Read:   true,
					Edit:   true,
					Remove: true,
					Drag:   true,
					DropOn: true,
				},
				// we don't know the block type for sure, lets set a page
				Content: &model.BlockContentOfPage{Page: &model.BlockContentPage{}},
			}},
	}
}
