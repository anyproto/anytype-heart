/*package block

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockUpdate(upd *pb.Event_BlockUpdate) {
	// BLOCK_UPDATED
	// m := &pb.Event{Message: &pb.Event_AccountShow{AccountShow: &pb.AccountShow{Index: int64(index), Account: account}}}

	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}
}

func (mw *Middleware) BlockRead(upd *pb.Event_BlockRead) {
	if mw.SendEvent != nil {
		mw.SendEvent(m)
	}
}
*/