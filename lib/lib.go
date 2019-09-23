package main

/*
#include <stdlib.h>
#include <stdint.h>
#include "bridge.h"
*/
import "C"

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/common/log"
)

var addonProxyFunc C.proxyFunc
var eventHandlerJsFunc unsafe.Pointer

//export SetProxyFunc
func SetProxyFunc(proxyFunc C.proxyFunc) {
	addonProxyFunc = proxyFunc
}

//export SetEventHandler
func SetEventHandler(jsFunc unsafe.Pointer) {
	eventHandlerJsFunc = jsFunc
}

//export Command
func Command(command *C.char, data unsafe.Pointer, dataLen C.int, callbackJsFunc unsafe.Pointer) {
	b := C.GoBytes(data, dataLen)
	// todo: free the pointer?

	cmd := C.GoString(command)
	var cd []byte
	switch cmd {
	case "WalletCreate":
		cd = walletCreate(b)
	default:
		fmt.Printf("unknown command type: %s\n", cmd)
	}

	if cd != nil {
		C.ProxyCall(addonProxyFunc, callbackJsFunc, C.CString(""), C.CString(string(cd)), C.int(len(cd)))
	}
	go func(){
		time.Sleep(time.Second*5)
		SendEvent(&pb.Event{
			Message: &pb.Event_AccountFound{
				&pb.AccountFound{
					Account: &pb.Account{
						Id: "testID",
						Name: "testName",
					},
				},
			},
		})
	}()
}

func SendEvent(event *pb.Event) {
	b, err := proto.Marshal(event)
	if err != nil {
		log.Errorf("failed to encode event: %s", err.Error())
		return
	}

	if eventHandlerJsFunc != nil {
		C.ProxyCall(addonProxyFunc, eventHandlerJsFunc, C.CString(""), C.CString(string(b)), C.int(len(b)))
	}
}

func main(){

}
