package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/ai/mock_ai"
)

type fixture struct {
	*AIService
	mockHttpClient *mock_ai.MockHttpClient
}

func newFixture(t *testing.T) *fixture {
	mockHttpClient := mock_ai.NewMockHttpClient(t)

	aiService := &AIService{
		httpClient: mockHttpClient,
		apiConfig: &APIConfig{
			Model:    "test-model",
			Endpoint: "http://example.com",
		},
		promptConfig: &PromptConfig{
			SystemPrompt: "system",
			UserPrompt:   "user",
			Temperature:  0.1,
			JSONMode:     false,
			Mode:         pb.RpcAIWritingToolsRequest_SUMMARIZE,
		},
	}

	return &fixture{
		AIService:      aiService,
		mockHttpClient: mockHttpClient,
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

	readLine := func() (string, error) {
		buf := make([]byte, 1024)
		n, err := psr.Read(buf)
		if err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	}

	t.Run("valid line with prefix", func(t *testing.T) {
		line, err := readLine()
		require.NoError(t, err)
		require.Equal(t, "{\"id\":\"test1\",\"content\":\"Hello!\"}\n", line)
	})

	t.Run("skips DONE line", func(t *testing.T) {
		line, err := readLine()
		require.NoError(t, err)
		require.Equal(t, "{\"id\":\"test2\",\"content\":\"How are you?\"}\n", line)
	})

	t.Run("no prefix line unchanged", func(t *testing.T) {
		line, err := readLine()
		require.NoError(t, err)
		require.Equal(t, "no prefix line\n", line)
	})

	t.Run("final prefixed line", func(t *testing.T) {
		line, err := readLine()
		require.NoError(t, err)
		require.Equal(t, "{\"id\":\"test3\",\"content\":\"Final line\"}\n", line)
	})

	t.Run("EOF after last line", func(t *testing.T) {
		_, err := readLine()
		require.Equal(t, io.EOF, err)
	})
}

func TestCreateChatRequest(t *testing.T) {
	t.Run("no json mode", func(t *testing.T) {
		fx := newFixture(t)

		data, err := fx.createChatRequest()
		require.NoError(t, err)

		var req ChatRequest
		err = json.Unmarshal(data, &req)
		require.NoError(t, err)

		require.Equal(t, "test-model", req.Model)
		require.Len(t, req.Messages, 2)
		require.Equal(t, "system", req.Messages[0]["content"])
		require.Equal(t, "user", req.Messages[1]["content"])
		require.Equal(t, float32(0.1), req.Temperature)
		require.True(t, req.Stream)
		require.Nil(t, req.ResponseFormat)
	})

	t.Run("json mode", func(t *testing.T) {
		fx := newFixture(t)

		fx.promptConfig.JSONMode = true

		data, err := fx.createChatRequest()
		require.NoError(t, err)

		var req ChatRequest
		err = json.Unmarshal(data, &req)
		require.NoError(t, err)

		require.NotNil(t, req.ResponseFormat)
		require.Equal(t, "json_schema", req.ResponseFormat["type"])

		schema, ok := req.ResponseFormat["json_schema"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "summary_response", schema["name"])
		require.Equal(t, true, schema["strict"])

		properties, ok := schema["schema"].(map[string]interface{})["properties"].(map[string]interface{})
		require.True(t, ok)
		require.Contains(t, properties, "summary")
		require.Equal(t, "string", properties["summary"].(map[string]interface{})["type"])
		require.Equal(t, false, schema["schema"].(map[string]interface{})["additionalProperties"])
		require.Equal(t, []interface{}{"summary"}, schema["schema"].(map[string]interface{})["required"])
	})
}

func TestChat(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fx := newFixture(t)

		responseData := `data: {"id":"test1","object":"chat","created":12345,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}
data: {"id":"test1","object":"chat","created":12346,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":" world!"}}]}
data: [DONE]
`
		resp := &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(responseData)),
		}
		fx.mockHttpClient.On("Do", mock.AnythingOfType("*http.Request")).Return(resp, nil)

		result, err := fx.chat(context.Background())
		require.NoError(t, err)
		require.Equal(t, "Hello world!", result)
		fx.mockHttpClient.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		fx := newFixture(t)

		resp := &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(strings.NewReader("not found")),
		}
		fx.mockHttpClient.On("Do", mock.AnythingOfType("*http.Request")).Return(resp, nil)

		_, err := fx.chat(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "model not found")
		fx.mockHttpClient.AssertExpectations(t)
	})

	t.Run("unauthorized", func(t *testing.T) {
		fx := newFixture(t)

		resp := &http.Response{
			StatusCode: 401,
			Body:       io.NopCloser(strings.NewReader("unauthorized")),
		}
		fx.mockHttpClient.On("Do", mock.AnythingOfType("*http.Request")).Return(resp, nil)

		_, err := fx.chat(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid for endpoint")
		fx.mockHttpClient.AssertExpectations(t)
	})
}

func TestParseChatResponse(t *testing.T) {
	t.Run("valid response", func(t *testing.T) {
		fx := newFixture(t)

		responseData := `data: {"id":"test1","object":"chat","created":12345,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello!"}}]}
data: {"id":"test2","object":"chat","created":12346,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"How are you?"}}]}
data: [DONE]
`
		responses, err := fx.parseChatResponse(strings.NewReader(responseData))
		require.NoError(t, err)
		require.Len(t, *responses, 2)
		require.Equal(t, "test1", (*responses)[0].ID)
		require.Equal(t, "Hello!", (*responses)[0].Choices[0].Delta.Content)
		require.Equal(t, "test2", (*responses)[1].ID)
		require.Equal(t, "How are you?", (*responses)[1].Choices[0].Delta.Content)
	})

	t.Run("invalid response", func(t *testing.T) {
		fx := newFixture(t)

		responseData := `data: {"id":"test1","object":"chat","created":12345,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello!"}}]}
data: {"id":"test2"  -- invalid json --
data: [DONE]
`
		responses, err := fx.parseChatResponse(strings.NewReader(responseData))
		require.Error(t, err)
		require.Nil(t, responses)
	})
}

func TestExtractAnswerByMode(t *testing.T) {
	t.Run("valid mode", func(t *testing.T) {
		fx := newFixture(t)

		fx.promptConfig.Mode = pb.RpcAIWritingToolsRequest_SUMMARIZE

		jsonData := `{"summary":"This is a summary"}`
		result, err := fx.extractAnswerByMode(jsonData)
		require.NoError(t, err)
		require.Equal(t, "This is a summary", result)
	})

	t.Run("empty response", func(t *testing.T) {
		fx := newFixture(t)

		fx.promptConfig.Mode = pb.RpcAIWritingToolsRequest_SUMMARIZE

		jsonData := `{"summary":""}`
		_, err := fx.extractAnswerByMode(jsonData)
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty")
	})

	t.Run("unknown mode", func(t *testing.T) {
		fx := newFixture(t)

		fx.promptConfig.Mode = pb.RpcAIWritingToolsRequestMode(9999)

		jsonData := `{}`
		_, err := fx.extractAnswerByMode(jsonData)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown mode")
	})
}
