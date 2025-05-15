package mcp

import "encoding/json"

type Request struct {
	JsonRpc string `json:"jsonrpc"`
	Id      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type ResponseRaw struct {
	Id     int             `json:"id,omitempty"`
	Result json.RawMessage `json:"result"`

	// Error
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type Response[T any] struct {
	Id     int `json:"id,omitempty"`
	Result T   `json:"result"`

	// Error
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

type Notification struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

type ToolsResponse struct {
	Tools []Tool `json:"tools"`
}

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

type ToolCallResult struct {
	IsError bool                    `json:"isError"`
	Content []ToolCallResultContent `json:"content"`
}

type ToolCallResultContent struct {
	Type string `json:"type"`
	Text string `json:"text"`

	// TODO: https://modelcontextprotocol.io/specification/2025-03-26/server/tools#tool-result
}
