package core

import (
	"context"
	"fmt"

	"github.com/textileio/go-threads/core/service"
	"github.com/textileio/go-threads/crypto/symmetric"

	"github.com/textileio/go-threads/core/thread"
)

var ErrorNoBlockVersionsFound = fmt.Errorf("no block versions found")

func (a *Anytype) newBlockThread(blockType SmartBlockType) (thread.Info, error) {
	thrdId, err := newThreadID(thread.AccessControlled, blockType)
	if err != nil {
		return thread.Info{}, err
	}
	followKey, err := symmetric.CreateKey()
	if err != nil {
		return thread.Info{}, err
	}

	readKey, err := symmetric.CreateKey()
	if err != nil {
		return thread.Info{}, err
	}

	return a.ts.CreateThread(context.TODO(), thrdId, service.FollowKey(followKey), service.ReadKey(readKey))
}

func (a *Anytype) GetSmartBlock(id string) (*smartBlock, error) {
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

	return &smartBlock{thread: thrd, node: a}, nil
}
