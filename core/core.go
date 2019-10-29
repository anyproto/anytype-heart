package core

import (
	"context"

	libCore "github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("anytype-mw")

func NewMiddleware() *Middleware {
	return &Middleware{
		linkPreview: linkpreview.New(),
	}
}

type Middleware struct {
	rootPath            string
	pin                 string
	mnemonic            string
	accountSearchCancel context.CancelFunc
	localAccounts       []*pb.Account
	SendEvent           func(event *pb.Event)
	linkPreview         linkpreview.LinkPreview
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
