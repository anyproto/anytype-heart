package main

import (
	"github.com/anyproto/anytype-heart/cmd/assistant/mcp"
)

func initMcpClients(dataDir string) ([]*mcp.Client, error) {
	testConfig := mcp.Config{
		Command: "npx",
		Args: []string{
			"-y", "@modelcontextprotocol/server-filesystem", "/Users/deff/dev/work/any-sync",
		},
	}

	client, err := mcp.New(testConfig)
	if err != nil {
		return nil, err
	}
	return []*mcp.Client{client}, nil
}
