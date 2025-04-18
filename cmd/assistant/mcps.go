package main

import (
	"github.com/anyproto/anytype-heart/cmd/assistant/mcp"
)

func initMcpClients(configs []mcp.Config) ([]*mcp.Client, error) {
	clients := make([]*mcp.Client, 0, len(configs))

	for _, cfg := range configs {
		client, err := mcp.New(cfg)
		if err != nil {
			return nil, err
		}
		clients = append(clients, client)
	}

	return clients, nil
}
