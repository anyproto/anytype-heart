//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pb"

	_ "net/http/pprof"
)

func main() {
	if len(os.Args) > 1 {
		for _, path := range os.Args[1:] {
			s, err := decodeFile(path)
			if err != nil {
				fmt.Printf("failed to decode %s: %s\n", path, err.Error())
				continue
			}
			fmt.Println(path + ":")
			fmt.Println(s)
			fmt.Print("\n\n")
		}
	}
}

func decodeFile(path string) (string, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %s", err)
	}
	var snapshot pb.ChangeSnapshot
	err = proto.Unmarshal(b, &snapshot)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal pb: %s", err)
	}
	marsh := &jsonpb.Marshaler{Indent: " "}
	s, err := marsh.MarshalToString(&snapshot)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to json: %s", err)
	}

	return s, nil
}
