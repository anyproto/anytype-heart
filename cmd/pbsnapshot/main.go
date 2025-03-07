//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package main

import (
	"fmt"
	"io/ioutil"
	_ "net/http/pprof"
	"os"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/anyproto/anytype-heart/pb"
)

func main() {
	if len(os.Args) > 1 {
		for _, path := range os.Args[1:] {
			s, err := decodeFile(path)
			if err != nil {
				fmt.Printf("failed to decode %s: %s\n", path, err)
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
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	var snapshot pb.ChangeSnapshot
	err = snapshot.UnmarshalVT(b)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal pb: %w", err)
	}
	marsh := &protojson.MarshalOptions{Indent: " "}
	s, err := marsh.Marshal(&snapshot)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to json: %w", err)
	}

	return string(s), nil
}
