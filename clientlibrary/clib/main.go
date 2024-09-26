package main

/*
#include <stdlib.h>
#include <stdint.h>
#include "bridge.h"
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/clientlibrary/service"
	"github.com/anyproto/anytype-heart/pb"
)

//export SetEventHandler
func SetEventHandler(pf C.proxyFunc, ctx unsafe.Pointer) {
	service.SetEventHandler(func(event *pb.Event) {
		if len(event.Messages) == 0 {
			return
		}
		b, err := proto.Marshal(event)
		if err != nil {
			fmt.Printf("failed to encode event: %s\n", err.Error())
			return
		}

		if pf != nil {
			C.ProxyCall(pf, ctx, C.CString(""), C.CString(string(b)), C.int(len(b)))
		} else {
			eventB, _ := json.Marshal(event)
			fmt.Printf("failed to send event to nil eventHandler: %s", string(eventB))
		}
	})
}

//export RunDebugServer
func RunDebugServer(addr *C.char) {
	service.RunDebugServer(C.GoString(addr))
}

//export Command
func Command(cmd *C.char, data unsafe.Pointer, dataLen C.int, callback C.proxyFunc, callbackContext unsafe.Pointer) {
	service.CommandAsync(C.GoString(cmd), C.GoBytes(data, dataLen), func(data []byte) {
		C.ProxyCall(callback, callbackContext, C.CString(""), C.CString(string(data)), C.int(len(data)))
	})
}

//export Shutdown
func Shutdown() {
	service.AppShutdown(nil)
}

func main() {

}
