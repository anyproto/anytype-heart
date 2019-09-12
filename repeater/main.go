package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"syscall"

	protoEV "../build/go"
	"github.com/golang/protobuf/proto"
)

var go_temp = "/var/tmp/.go_pipe"
var js_temp = "/var/tmp/.js_pipe"

func main() {
	syscall.Mkfifo(go_temp, 0600)
	f, err := os.OpenFile(go_temp, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}

	reader(f)
}

func reader(f *os.File) {
	file, err := os.OpenFile(js_temp, os.O_CREATE, os.ModeNamedPipe)
	if err != nil {
		log.Fatal("Open named pipe file error:", err)
	}

	reader := bufio.NewReader(file)

	for {
		line, err := reader.ReadBytes('\n')
		if err == nil {
			data, err := base64.StdEncoding.DecodeString(string(line))
			if err != nil {
				fmt.Println("error DecodeString:", err)
			}

			event := &protoEV.Event{}
			err = proto.Unmarshal(data, event)
			if err != nil {
				fmt.Println("error Unmarshal:", err)
			}

			fmt.Println("event:", event)
			f.WriteString(string(line))
		}
	}
}
