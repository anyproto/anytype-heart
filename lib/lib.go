package main

/*
#include <stdlib.h>
#include <stdint.h>
#include "bridge.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"unsafe"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	log "github.com/sirupsen/logrus"
)

var eventHandler func(event *pb.Event)

type Instance struct {
	rootPath            string
	pin                 string
	mnemonic            string
	accountSearchCancel context.CancelFunc
	localAccounts       []*pb.Account
	*core.Anytype
}

var instance = &Instance{}

//export SetCEventHandler
func SetCEventHandler(proxyFunc C.proxyFunc, ctx unsafe.Pointer) {
	SetEventHandler(func(event *pb.Event){
		b, err := proto.Marshal(event)
		if err != nil {
			log.Errorf("failed to encode event: %s", err.Error())
			return
		}

		if proxyFunc != nil {
			C.ProxyCall(proxyFunc, ctx, C.CString(""), C.CString(string(b)), C.int(len(b)))
		} else {
			eventB, _ := json.Marshal(event)
			log.Errorf("failed to send event to nil eventHandler: %s", string(eventB))
		}
	})
}

func SetEventHandler(eh func(event *pb.Event)){
	eventHandler = eh
}

func command(cmd string, data []byte, callback func(data []byte)) {
	go func() {
		var cd []byte
		switch cmd {
			case "WalletCreate":
				cd = WalletCreate(data)
			case "WalletRecover":
				cd = WalletRecover(data)
			case "AccountCreate":
				cd = AccountCreate(data)
			case "AccountSelect":
				cd = AccountSelect(data)
			case "ImageGetBlob":
				cd = ImageGetBlob(data)
			default:
				fmt.Printf("unknown command type: %s\n", cmd)
		}

		callback(cd)
	}()
}

//export Command
func Command(cmd *C.char, data unsafe.Pointer, dataLen C.int, callback C.proxyFunc, callbackContext unsafe.Pointer) {
	command(C.GoString(cmd), C.GoBytes(data, dataLen), func(data []byte) {
		C.ProxyCall(callback, callbackContext, C.CString(""), C.CString(string(data)), C.int(len(data)))
	})
}

func SendEvent(event *pb.Event) {
	if eventHandler == nil {
		b, _ := json.Marshal(event)
		log.Errorf("failed to send event to nil eventHandler: %s", string(b))
		return
	}

	eventHandler(event)
}

func (instnc *Instance) Stop() error {
	if instnc != nil && instance.Anytype != nil {
		err := instnc.Anytype.Stop()
		if err != nil {
			return err
		}

		instnc.Anytype = nil
		instnc.accountSearchCancel = nil
	}

	return nil
}

func main() {

}
