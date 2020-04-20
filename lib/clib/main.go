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

	"github.com/anytypeio/go-anytype-middleware/lib"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
)

//export SetEventHandler
func SetEventHandler(proxyFunc C.proxyFunc, ctx unsafe.Pointer) {
	lib.SetEventHandler(func(event *pb.Event) {
		b, err := proto.Marshal(event)
		if err != nil {
			fmt.Printf("failed to encode event: %s\n", err.Error())
			return
		}

		if proxyFunc != nil {
			C.ProxyCall(proxyFunc, ctx, C.CString(""), C.CString(string(b)), C.int(len(b)))
		} else {
			eventB, _ := json.Marshal(event)
			fmt.Printf("failed to send event to nil eventHandler: %s", string(eventB))
		}
	})
}

//export Command
func Command(cmd *C.char, data unsafe.Pointer, dataLen C.int, callback C.proxyFunc, callbackContext unsafe.Pointer) {
	lib.CommandAsync(C.GoString(cmd), C.GoBytes(data, dataLen), func(data []byte) {
		C.ProxyCall(callback, callbackContext, C.CString(""), C.CString(string(data)), C.int(len(data)))
	})
}

func main(){

}
