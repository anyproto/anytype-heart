package core

import (
	"context"

	libCore "github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("anytype-mw")

type Middleware struct {
	rootPath            string
	pin                 string
	mnemonic            string
	accountSearchCancel context.CancelFunc
	localAccounts       []*pb.Account
	SendEvent           func(event *pb.Event)
	*libCore.Anytype
}

func (mw *Middleware) Stop() error {
	if mw != nil && mw.Anytype != nil {
		err := mw.Anytype.Stop()
		if err != nil {
			return err
		}

		mw.Anytype = nil
		mw.accountSearchCancel = nil
	}

	return nil
}
