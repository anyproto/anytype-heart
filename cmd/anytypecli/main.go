package main

import (
	"github.com/anyproto/anytype-heart/cmd/anytypecli/cmd"
	"github.com/anyproto/anytype-heart/cmd/anytypecli/process"
)

func main() {
	defer process.CloseGRPCConnection()
	cmd.Execute()
}
