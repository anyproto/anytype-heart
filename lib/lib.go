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
	"github.com/requilence/go-anytype/pb"
)

//export SetFunc
func SetFunc(f C.voidFunc) {
	fmt.Println("SetFunc called")
	C.setFunc(f)
}

//export Call
func Call(methodC *C.char, data unsafe.Pointer, dataLen C.int) {
	b := C.GoBytes(data, dataLen)

	method := C.GoString(methodC)
	switch method {
	case "GenerateMnemonic":
		generateMnemonic(b)
	default:
		fmt.Printf("unknown method: %s\n",method)
	}
}

func CallbackError(err error){
	// todo: wrap error to add a code
	b, _ := proto.Marshal(&pb.Error{Error: err.Error()})

	C.CallFunction(C.CString("ShowError"), C.CString(string(b)), C.int(len(b)))
}

func Callback(method string, data proto.Message){
	b, err := proto.Marshal(data)
	if err != nil {
		CallbackError(err)
		return
	}

	fmt.Printf("Callback(%s, %x, %d)\n", method, b, len(b))
	C.CallFunction(C.CString(method), C.CString(string(b)), C.int(len(b)))
}

func main(){

}
