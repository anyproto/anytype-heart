package ai

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
)

func TestWritingTools(t *testing.T) {
	err := godotenv.Load() // Ensure dotenv is loaded for tests needing API keys
	assert.NoError(t, err)

	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	if openaiAPIKey == "" {
		log.Warn("OPENAI_API_KEY not found in environment, using default invalid token")
		openaiAPIKey = "default-invalid-token" // Fallback in case of missing API key
	}

	tests := []struct {
		name           string
		params         *pb.RpcAIWritingToolsRequest
		validateResult func(t *testing.T, result Result, err error)
	}{
		{
			name: "SupportedLanguage",
			params: &pb.RpcAIWritingToolsRequest{
				Mode:        0,
				Language:    0,
				Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
				Endpoint:    "http://localhost:11434/v1/chat/completions",
				Model:       "llama3.2:3b",
				Token:       "",
				Temperature: 0.1,
				Text:        "This is a test.",
			},
			validateResult: func(t *testing.T, result Result, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "UnsupportedLanguage",
			params: &pb.RpcAIWritingToolsRequest{
				Mode:        0,
				Language:    0,
				Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
				Endpoint:    "http://localhost:11434/v1/chat/completions",
				Model:       "llama3.2:3b",
				Token:       "",
				Temperature: 0.1,
				Text:        "Съешь ещё этих мягких французских булок, да выпей же чаю. Впрочем, одних слов недостаточно для демонстрации эффекта, но этот текст подходит для большинства задач. Здесь можно написать что угодно, и никто не обратит внимания на содержание.",
			},
			validateResult: func(t *testing.T, result Result, err error) {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, ErrUnsupportedLanguage))
			},
		},
		{
			name: "InvalidEndpoint",
			params: &pb.RpcAIWritingToolsRequest{
				Mode:        0,
				Language:    0,
				Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
				Endpoint:    "http://invalid-endpoint",
				Model:       "llama3.2:3b",
				Token:       "",
				Temperature: 0.1,
				Text:        "Can you use an invalid endpoint?",
			},
			validateResult: func(t *testing.T, result Result, err error) {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, ErrEndpointNotReachable))
			},
		},
		{
			name: "InvalidModel",
			params: &pb.RpcAIWritingToolsRequest{
				Mode:        0,
				Language:    0,
				Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
				Endpoint:    "http://localhost:11434/v1/chat/completions",
				Model:       "invalid-model",
				Token:       "",
				Temperature: 0.1,
				Text:        "Can you use an invalid model?",
			},
			validateResult: func(t *testing.T, result Result, err error) {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, ErrModelNotFound))
			},
		},
		{
			name: "UnauthorizedAccess",
			params: &pb.RpcAIWritingToolsRequest{
				Mode:        0,
				Language:    0,
				Provider:    pb.RpcAIWritingToolsRequest_OPENAI,
				Endpoint:    "https://api.openai.com/v1/chat/completions",
				Model:       "gpt-4o-mini",
				Token:       "invalid-token",
				Temperature: 0.1,
				Text:        "Can you use an invalid token?",
			},
			validateResult: func(t *testing.T, result Result, err error) {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, ErrAuthRequired))
			},
		},
		{
			name: "ValidResponseOllama",
			params: &pb.RpcAIWritingToolsRequest{
				Mode:        0,
				Language:    0,
				Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
				Endpoint:    "http://localhost:11434/v1/chat/completions",
				Model:       "llama3.2:3b",
				Token:       "",
				Temperature: 0,
				Text:        "What is the capital of France?",
			},
			validateResult: func(t *testing.T, result Result, err error) {
				assert.NoError(t, err)
				assert.NotEmpty(t, result.Answer)
				assert.Contains(t, result.Answer, "Paris")
			},
		},
		{
			name: "ValidResponseOpenAI",
			params: &pb.RpcAIWritingToolsRequest{
				Mode:        0,
				Language:    0,
				Provider:    pb.RpcAIWritingToolsRequest_OPENAI,
				Endpoint:    "https://api.openai.com/v1/chat/completions",
				Model:       "gpt-4o-mini",
				Token:       openaiAPIKey,
				Temperature: 0,
				Text:        "What is the capital of France?",
			},
			validateResult: func(t *testing.T, result Result, err error) {
				assert.NoError(t, err)
				assert.NotEmpty(t, result.Answer)
				assert.Contains(t, result.Answer, "Paris")
			},
		},
		{
			name: "JSONExtraction",
			params: &pb.RpcAIWritingToolsRequest{
				Mode:        6,
				Language:    0,
				Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
				Endpoint:    "http://localhost:11434/v1/chat/completions",
				Model:       "llama3.2:3b",
				Token:       "",
				Temperature: 0,
				Text:        "Countries, Capitals\nFrance, Paris\nGermany, Berlin",
			},
			validateResult: func(t *testing.T, result Result, err error) {
				assert.NoError(t, err)
				assert.NotEmpty(t, result.Answer)
				assert.Equal(t, "| Country | Capital |\n|----------|---------|\n| France   | Paris   |\n| Germany  | Berlin  |\n", result.Answer)
			},
		},
	}

	service := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.WritingTools(context.Background(), tt.params)
			tt.validateResult(t, result, err)
		})
	}
}
