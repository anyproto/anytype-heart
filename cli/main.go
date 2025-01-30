package main

import (
	"github.com/anyproto/anytype-heart/cli/cmd"
	"github.com/anyproto/anytype-heart/cli/internal"
)

func main() {
	defer internal.CloseGRPCConnection()
	cmd.Execute()
}
