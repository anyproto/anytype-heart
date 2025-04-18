package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

var testConfig = Config{
	Command: "npx",
	Args: []string{
		"-y", "@modelcontextprotocol/server-filesystem", "/Users/deff/dev/work/any-sync",
	},
}

func TestClient(t *testing.T) {
	c, err := New(testConfig)
	require.NoError(t, err)
	defer c.Close()

	err = c.Initialize(context.Background())
	require.NoError(t, err)

	c.ListTools()
	_ = c
}
