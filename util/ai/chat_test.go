package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/ai/mock_ai"
)

func newTestAIService(mockClient *mock_ai.MockHttpClient) *AIService {
	return &AIService{
		httpClient: mockClient,
	}
}

func TestPrefixStrippingReader(t *testing.T) {
	prefix := "data: "
	input := `data: {"id":"test1","content":"Hello!"}
data: [DONE]
data: {"id":"test2","content":"How are you?"}
no prefix line
data: [DONE]
data: {"id":"test3","content":"Final line"}
`

	psr := &prefixStrippingReader{
		reader: bufio.NewReader(strings.NewReader(input)),
		prefix: prefix,
	}

	// Helper to read one line at a time using psr.Read
	readLine := func() (string, error) {
		buf := make([]byte, 1024)
		n, err := psr.Read(buf)
		if err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	}

	// 1. First call should strip the "data: " prefix
	line, err := readLine()
	assert.NoError(t, err)
	assert.Equal(t, "{\"id\":\"test1\",\"content\":\"Hello!\"}\n", line)

	// 2. Next line is "data: [DONE]" which should be skipped entirely.
	//    The next non-[DONE] line is "data: {"id":"test2","content":"How are you?"}"
	line, err = readLine()
	assert.NoError(t, err)
	assert.Equal(t, "{\"id\":\"test2\",\"content\":\"How are you?\"}\n", line)

	// 3. A line that doesn't start with the prefix remains the same
	line, err = readLine()
	assert.NoError(t, err)
	assert.Equal(t, "no prefix line\n", line)

	// 4. Another [DONE] line should be skipped, then the final prefixed line is returned
	line, err = readLine()
	assert.NoError(t, err)
	assert.Equal(t, "{\"id\":\"test3\",\"content\":\"Final line\"}\n", line)

	// 5. Attempting to read again should return EOF
	_, err = readLine()
	assert.Equal(t, io.EOF, err)
}

func TestCreateChatRequest(t *testing.T) {
	aiService := newTestAIService(nil)
	aiService.apiConfig = &APIConfig{
		Model:    "test-model",
		Endpoint: "http://example.com",
	}
	aiService.promptConfig = &PromptConfig{
		SystemPrompt: "system",
		UserPrompt:   "user",
		Temperature:  0.7,
		JSONMode:     false,
		Mode:         pb.RpcAIWritingToolsRequest_SUMMARIZE,
	}

	data, err := aiService.createChatRequest()
	require.NoError(t, err)

	var req ChatRequest
	err = json.Unmarshal(data, &req)
	require.NoError(t, err)
	assert.Equal(t, "test-model", req.Model)
	assert.Len(t, req.Messages, 2)
	assert.Equal(t, "system", req.Messages[0]["content"])
	assert.Equal(t, "user", req.Messages[1]["content"])
	assert.Equal(t, float32(0.7), req.Temperature)
	assert.True(t, req.Stream)
	assert.Nil(t, req.ResponseFormat)
}

func TestCreateChatRequest_JSONMode(t *testing.T) {
	aiService := newTestAIService(nil)
	aiService.apiConfig = &APIConfig{
		Model:    "test-model",
		Endpoint: "http://example.com",
	}
	aiService.promptConfig = &PromptConfig{
		SystemPrompt: "system",
		UserPrompt:   "user",
		Temperature:  0.7,
		JSONMode:     true,
		Mode:         pb.RpcAIWritingToolsRequest_SUMMARIZE,
	}

	data, err := aiService.createChatRequest()
	require.NoError(t, err)

	var req ChatRequest
	err = json.Unmarshal(data, &req)
	require.NoError(t, err)

	assert.NotNil(t, req.ResponseFormat)
	assert.Equal(t, "json_schema", req.ResponseFormat["type"])
}

func TestParseChatResponse_Valid(t *testing.T) {
	aiService := newTestAIService(nil)

	responseData := `data: {"id":"test1","object":"chat","created":12345,"model":"test-model","system_fingerprint":"fp","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello!"}}]}
data: {"id":"test2","object":"chat","created":12346,"model":"test-model","system_fingerprint":"fp","choices":[{"index":0,"delta":{"role":"assistant","content":"How are you?"}}]}
data: [DONE]
`

	responses, err := aiService.parseChatResponse(strings.NewReader(responseData))
	require.NoError(t, err)
	require.Len(t, *responses, 2)
	assert.Equal(t, "test1", (*responses)[0].ID)
	assert.Equal(t, "Hello!", (*responses)[0].Choices[0].Delta.Content)
	assert.Equal(t, "test2", (*responses)[1].ID)
	assert.Equal(t, "How are you?", (*responses)[1].Choices[0].Delta.Content)
}

func TestParseChatResponse_InvalidJSON(t *testing.T) {
	aiService := newTestAIService(nil)

	responseData := `data: {"id":"test1","object":"chat","created":12345,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello!"}}]}
data: {"id":"test2"  -- invalid json --
data: [DONE]
`

	responses, err := aiService.parseChatResponse(strings.NewReader(responseData))
	assert.Error(t, err)
	assert.Nil(t, responses)
}

func TestChat_Success(t *testing.T) {
	mockClient := &mock_ai.MockHttpClient{}

	responseData := `data: {"id":"test1","object":"chat","created":12345,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}
data: {"id":"test1","object":"chat","created":12346,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":" world!"}}]}
data: [DONE]
`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(responseData)),
	}

	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(resp, nil)

	aiService := newTestAIService(mockClient)
	aiService.apiConfig = &APIConfig{
		Model:    "test-model",
		Endpoint: "http://example.com",
	}
	aiService.promptConfig = &PromptConfig{
		SystemPrompt: "system",
		UserPrompt:   "user",
		Temperature:  0.7,
		JSONMode:     false,
		Mode:         pb.RpcAIWritingToolsRequest_SUMMARIZE,
	}

	result, err := aiService.chat(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Hello world!", result)

	mockClient.AssertExpectations(t)
}

func TestChat_Non200Status(t *testing.T) {
	mockClient := &mock_ai.MockHttpClient{}

	resp := &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(strings.NewReader("not found")),
	}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(resp, nil)

	aiService := newTestAIService(mockClient)
	aiService.apiConfig = &APIConfig{
		Model:    "test-model",
		Endpoint: "http://example.com",
	}
	aiService.promptConfig = &PromptConfig{}

	_, err := aiService.chat(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model not found")

	mockClient.AssertExpectations(t)
}

func TestExtractAnswerByMode(t *testing.T) {
	aiService := newTestAIService(nil)
	aiService.promptConfig = &PromptConfig{
		Mode: pb.RpcAIWritingToolsRequest_SUMMARIZE,
	}

	jsonData := `{"summary":"This is a summary","corrected":"This is corrected"}`
	result, err := aiService.extractAnswerByMode(jsonData)
	require.NoError(t, err)
	assert.Equal(t, "This is a summary", result)
}

func TestExtractAnswerByMode_Empty(t *testing.T) {
	aiService := newTestAIService(nil)
	aiService.promptConfig = &PromptConfig{
		Mode: pb.RpcAIWritingToolsRequest_SUMMARIZE,
	}

	jsonData := `{"summary":""}`
	_, err := aiService.extractAnswerByMode(jsonData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestExtractAnswerByMode_UnknownMode(t *testing.T) {
	aiService := newTestAIService(nil)
	aiService.promptConfig = &PromptConfig{
		Mode: pb.RpcAIWritingToolsRequestMode(9999),
	}

	jsonData := `{}`
	_, err := aiService.extractAnswerByMode(jsonData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown mode")
}
