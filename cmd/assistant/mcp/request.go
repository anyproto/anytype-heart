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
