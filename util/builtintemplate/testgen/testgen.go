package main

import (
	"encoding/hex"
	"fmt"
)

var templatesBinary [][]byte

func main() {
	for _, tb := range templatesBinary {
		fmt.Println(hex.EncodeToString(tb))
	}
}
