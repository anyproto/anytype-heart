package main

/*
#include <stdlib.h>
#include <stdint.h>
#include "bridge.h"
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/gogo/protobuf/proto"
	"github.com/anytypeio/go-anytype-middleware/pb"
	log "github.com/sirupsen/logrus"
)

//export SetClientFunc
func SetClientFunc(f C.voidFunc) {
	C.setClientFunc(f)
}

//export Call
func Call(_ *C.char, data unsafe.Pointer, dataLen C.int) {
	b := C.GoBytes(data, dataLen)
	// todo: free the pointer?
	var msg pb.Client

	err := proto.Unmarshal(b, &msg)
	if err != nil {
		log.Errorf("unmarshal failed: %s", err.Error())
		CallClientWithStatus(&pb.Status{
			ReplyTo: msg.Id,
			Description: err.Error(),
			Status: &pb.Status_ArgError{pb.Status_DESERIALIZATION_FAILED},
		})
		return
	}
	switch v := msg.Event.(type) {
	case *pb.Client_WalletCreate:
		walletCreate(msg.Id, v.WalletCreate)
	default:
		fmt.Printf("unknown type: %T\n", v)
	}
}

func CallClientWithStatus(status *pb.Status){
	// todo: wrap error to add a code
	var msg = pb.Middle{
		Id: RandStringRunes(6),
		Message: &pb.Middle_Status{status},
	}
	CallClient(&msg)
}

func CallClient(msg *pb.Middle){
	msg.Id = RandStringRunes(6)
	b, err := proto.Marshal(msg)
	if err != nil {
		CallClientWithStatus(&pb.Status{
			Description: err.Error(),
			Status: &pb.Status_IntError{pb.Status_INTERNAL_ERROR},
		})
		return
	}

	C.CallClientFunc(C.CString(""), C.CString(string(b)), C.int(len(b)))
}


func main(){

}
