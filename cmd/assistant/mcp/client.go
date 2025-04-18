package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("mcp-client").Desugar()

type Config struct {
	Command  string            `json:"command"`
	Args     []string          `json:"args"`
	Env      map[string]string `json:"env"`
	Disabled bool              `json:"disabled"`
}

type Client struct {
	config Config

	input  io.WriteCloser
	output io.ReadCloser
	cmd    *exec.Cmd

	cancel context.CancelFunc

	idCounter    atomic.Int64
	lock         *sync.Mutex
	notifierCond *sync.Cond
	responses    map[int]*ResponseRaw
}

// TODO Create response buffer

func New(config Config) (*Client, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, config.Command, config.Args...)

	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	lock := &sync.Mutex{}
	c := &Client{
		config:       config,
		input:        input,
		output:       output,
		cmd:          cmd,
		cancel:       cancel,
		responses:    make(map[int]*ResponseRaw),
		lock:         lock,
		notifierCond: sync.NewCond(lock),
	}

	err = cmd.Start()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start command: %w", err)
	}

	go func() {
		err = c.responseReader()
		if err != nil {
			log.Error("response reader", zap.Error(err))
		}
	}()

	return c, nil
}

func (c *Client) Close() error {
	c.cancel()
	return c.cmd.Wait()
}

func (c *Client) responseReader() error {
	scanner := bufio.NewScanner(c.output)
	for scanner.Scan() {
		raw := scanner.Bytes()
		var resp ResponseRaw
		err := json.Unmarshal(raw, &resp)
		if err != nil {
			log.Error("unmarshal response", zap.Error(err))
			continue
		}
		c.addResponse(&resp)
	}
	return scanner.Err()
}

func (c *Client) addResponse(resp *ResponseRaw) {
	c.notifierCond.L.Lock()
	defer c.notifierCond.L.Unlock()

	c.responses[resp.Id] = resp
	c.notifierCond.Broadcast()
}

// waitResponse waits for response and consumes it. The response could be only read once
func (c *Client) waitResponse(id int) *ResponseRaw {
	c.notifierCond.L.Lock()

	for {
		resp, ok := c.responses[id]
		if ok {
			delete(c.responses, id)
			c.notifierCond.L.Unlock()
			return resp
		} else {
			c.notifierCond.Wait()
		}
	}
}

func (c *Client) Initialize(ctx context.Context) error {
	resp, err := c.sendRequest(Request{
		Method: "initialize",
		Params: map[string]any{
			"capabilities": map[string]any{},
			"clientInfo": map[string]any{
				"name":    "anytype-assistant",
				"version": "0.1",
			},
			"protocolVersion": "2025-03-26",
		},
	})
	if err != nil {
		return fmt.Errorf("send initialize request: %w", err)
	}
	_ = resp

	// TODO check capabilities->tools

	err = c.sendNotification(Request{
		Method: "notifications/initialized",
	})
	if err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}

	return nil
}

func (c *Client) ListTools() ([]Tool, error) {
	resp, err := sendRequest[ToolsResponse](c, Request{
		Method: "tools/list",
	})
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	return resp.Result.Tools, nil
}

func (c *Client) CallTool(name string, params any) (*ToolCallResult, error) {
	resp, err := sendRequest[ToolCallResult](c, Request{
		Method: "tools/call",
		Params: map[string]any{
			"name":      name,
			"arguments": params,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("call tool %s: %w", name, err)
	}

	return &resp.Result, nil
}

func (c *Client) sendNotification(req Request) error {
	req.Id = 0
	return c.sendRaw(req)
}

func sendRequest[T any](c *Client, req Request) (*Response[T], error) {
	respRaw, err := c.sendRequest(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	resp := &Response[T]{
		Id:      respRaw.Id,
		Code:    respRaw.Code,
		Message: respRaw.Message,
		Data:    respRaw.Data,
	}

	err = json.Unmarshal(respRaw.Result, &resp.Result)
	if err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return resp, nil
}

func (c *Client) sendRequest(req Request) (*ResponseRaw, error) {
	req.Id = int(c.idCounter.Add(1))
	err := c.sendRaw(req)
	if err != nil {
		return nil, fmt.Errorf("send raw: %w", err)
	}
	resp := c.waitResponse(req.Id)
	if resp.Code != 0 {
		return nil, fmt.Errorf("response code=%d message=%s", resp.Code, resp.Message)
	}
	return resp, nil
}

func (c *Client) sendRaw(req Request) error {
	req.JsonRpc = "2.0"

	raw, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	_, err = c.input.Write(raw)
	if err != nil {
		return fmt.Errorf("write request: %w", err)
	}
	_, err = c.input.Write([]byte{'\n'})
	if err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	return nil
}
