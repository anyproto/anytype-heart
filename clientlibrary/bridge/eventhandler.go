//go:build cshared

package main

/*
#include <stdlib.h>

typedef void (*EventHandlerCallbackType)(unsigned char* data, int length);

static inline void callEventCallback(EventHandlerCallbackType callback, unsigned char* data, int length) {
    if (callback != NULL) {
        callback(data, length);
    }
}
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
func SetEventHandler(callback C.EventHandlerCallbackType) {
	service.SetEventHandler(func(event *pb.Event) {
		if len(event.Messages) == 0 {
			return
		}
		b, err := proto.Marshal(event)
		if err != nil {
			fmt.Printf("failed to encode event: %s\n", err.Error())
			return
		}

		if callback != nil {
			// Allocate memory in C heap and copy Go bytes to it
			cBytes := C.CBytes(b)
			C.callEventCallback(callback, (*C.uchar)(cBytes), C.int(len(b)))
			C.free(unsafe.Pointer(cBytes))
		} else {
			eventB, _ := json.Marshal(event)
			fmt.Printf("failed to send event to nil eventHandler: %s", string(eventB))
		}
	})
}
