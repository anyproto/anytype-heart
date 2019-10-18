package lib

import (
	"context"
	"encoding/json"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("anytype-mw")

type middleware struct {
	rootPath            string
	pin                 string
	mnemonic            string
	accountSearchCancel context.CancelFunc
	localAccounts       []*pb.Account
	*core.Anytype
}

var mw = &middleware{}
var eventHandler func(event *pb.Event)

type MessageHandler interface {
	Handle(b []byte)
}

func SetEventHandler(eh func(event *pb.Event)) {
	eventHandler = eh
}

func SetEventHandlerMobile(eh MessageHandler) {
	SetEventHandler(func(event *pb.Event) {
		b, err := proto.Marshal(event)
		if err != nil {
			log.Errorf("eventHandler failed to marshal error: %s", err.Error())
		}
		eh.Handle(b)
	})
}

func CommandAsync(cmd string, data []byte, callback func(data []byte)) {
	go func() {
		var cd []byte
		switch cmd {
		case "WalletCreate":
			cd = WalletCreate(data)
		case "WalletRecover":
			cd = WalletRecover(data)
		case "AccountCreate":
			cd = AccountCreate(data)
		case "AccountRecover":
			cd = AccountRecover(data)
		case "AccountSelect":
			cd = AccountSelect(data)
		case "ImageGetBlob":
			cd = ImageGetBlob(data)
		case "GetVersion":
			cd = GetVersion(data)
		case "Log":
			cd = Log(data)
		default:
			log.Errorf("unknown command type: %s\n", cmd)
		}

		callback(cd)
	}()
}

func CommandMobile(cmd string, data []byte, callback MessageHandler) {
	CommandAsync(cmd, data, callback.Handle)
}

func (mw *middleware) Stop() error {
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

func SendEvent(event *pb.Event) {
	if eventHandler == nil {
		b, _ := json.Marshal(event)
		log.Errorf("failed to send event to nil eventHandler: %s", string(b))
		return
	}

	eventHandler(event)
}
