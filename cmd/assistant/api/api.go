package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sashabaranov/go-openai"

	"github.com/anyproto/anytype-heart/cmd/assistant/mcp"
)

// APIClient manages API requests to the Anytype API
type APIClient struct {
	BaseURL    string
	HTTPClient *http.Client
	APIKey     string
	SpaceId    string
}

// NewAPIClient creates a new client for interacting with the Anytype API
func NewAPIClient(baseURL string, apiKey string, spaceId string) *APIClient {
	return &APIClient{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{},
		APIKey:     apiKey,
		SpaceId:    spaceId,
	}
}

// HandleToolCall processes a tool call and executes the corresponding API request
func (c *APIClient) HandleToolCall(tool ApiTool, args map[string]interface{}) (map[string]interface{}, error) {
	// Parse the function arguments
	args["space_id"] = c.SpaceId
	// Extract path parameters and build the URL
	url := c.buildURL(tool.Path, args)

	fmt.Printf("api request %s %s: %+v\n", tool.Method, url, args)
	// Create the request
	var reqBody io.Reader
	if tool.Method == "POST" || tool.Method == "PUT" || tool.Method == "PATCH" {
		jsonBody, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
		// Remove body from args so it doesn't get added as a query parameter
		args = map[string]interface{}{}
	}

	// Create HTTP request
	req, err := http.NewRequest(tool.Method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	req.Header.Set("Anytype-Version", "2025-03-17") // Use the current API version

	// Add query parameters
	q := req.URL.Query()
	for k, v := range args {
		// Skip parameters that have been used in the path
		if !strings.Contains(tool.Path, "{"+k+"}") {
			q.Add(k, fmt.Sprintf("%v", v))
		}
	}
	req.URL.RawQuery = q.Encode()

	// Execute the request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		fmt.Printf("api request %s %s: %v\n", req.Method, req.URL.String(), err)
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("api request %s %s: failed to read response body: %v\n", req.Method, req.URL.String(), err)
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Parse the response
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("api request %s %s: failed to parse response body: '%v'\n", req.Method, req.URL.String(), err, string(respBody))
		fmt.Printf("Response body: %s\n", string(respBody))
		// If response is not valid JSON, return as plain text
		return map[string]interface{}{
			"status_code": resp.StatusCode,
			"content":     string(respBody),
		}, nil
	}
	fmt.Printf("api request %s %s: %+v %s\n", req.Method, req.URL.String(), args, string(respBody))

	// Add status code to the result
	result["status_code"] = resp.StatusCode
	return result, nil
}

// CallTool implements the ToolCaller interface, allowing the APIClient to be used in
// place of an MCP client. It converts the generic parameters into a proper openai.ToolCall
// and uses HandleToolCall to process the request.
func (c *APIClient) CallTool(name string, params any) (*mcp.ToolCallResult, error) {
	// Create a function arguments string from the params
	jsonArgs, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %v", err)
	}

	// Create a tool call object to pass to HandleToolCall
	toolCall := openai.ToolCall{
		Type: openai.ToolTypeFunction,
		Function: openai.FunctionCall{
			Name:      name,
			Arguments: string(jsonArgs),
		},
	}

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return nil, fmt.Errorf("failed to parse function arguments: %v", err)
	}

	// Extract function name which contains method and path
	// Format is "METHOD /path/to/resource"
	functionName := toolCall.Function.Name
	tool := GetToolByName(functionName)
	// Process the tool call using the existing handler
	result, err := c.HandleToolCall(*tool, args)
	if err != nil {
		return &mcp.ToolCallResult{
			IsError: true,
			Content: []mcp.ToolCallResultContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Error: %v", err),
				},
			},
		}, nil
	}

	// Convert the result to a ToolCallResult
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &mcp.ToolCallResult{
			IsError: true,
			Content: []mcp.ToolCallResultContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Error marshaling result: %v", err),
				},
			},
		}, nil
	}

	return &mcp.ToolCallResult{
		IsError: false,
		Content: []mcp.ToolCallResultContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// buildURL constructs the full URL for the API request, including path parameters
func (c *APIClient) buildURL(path string, args map[string]interface{}) string {
	url := c.BaseURL
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += strings.TrimPrefix(path, "/")

	// Replace path parameters
	for k, v := range args {
		placeholder := "{" + k + "}"
		if strings.Contains(url, placeholder) {
			url = strings.ReplaceAll(url, placeholder, fmt.Sprintf("%v", v))
		}
	}

	return url
}
