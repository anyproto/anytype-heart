package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"syscall"
	"time"

	protoEV "../protocol/build/go/event"
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
	// go scheduleWriter(f)
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

func write(f *os.File, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	f.WriteString(string(encoded) + "\n")
}

func scheduleWriter(f *os.File) {
	fmt.Println("start schedule writing.")

	for {
		event := &protoEV.Event{
			Entity: "document",
			Op:     "newBlock",
			Data:   "0x123132",
			Id:     "123456789",
		}

		data, err := proto.Marshal(event)
		if err != nil {
			fmt.Println("error Unmarshal:", err)
		}

		write(f, data)
		time.Sleep(time.Second)
	}
}
