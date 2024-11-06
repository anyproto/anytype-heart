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

func TestWritingTools_SupportedLanguage(t *testing.T) {
	service := New()
	params := &pb.RpcAIWritingToolsRequest{
		Mode:        0,
		Language:    0,
		Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
		Endpoint:    "http://localhost:11434/v1/chat/completions",
		Model:       "llama3.2:3b",
		Token:       "",
		Temperature: 0.1,
		Text:        "This is a test.",
	}

	_, err := service.WritingTools(context.Background(), params)
	assert.NoError(t, err)
}

func TestWritingTools_UnsupportedLanguage(t *testing.T) {
	service := New()
	params := &pb.RpcAIWritingToolsRequest{
		Mode:        0,
		Language:    0,
		Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
		Endpoint:    "http://localhost:11434/v1/chat/completions",
		Model:       "llama3.2:3b",
		Token:       "",
		Temperature: 0.1,
		Text:        "Это тест.",
	}

	_, err := service.WritingTools(context.Background(), params)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnsupportedLanguage))
}

func TestWritingTools_ValidResponseOllama(t *testing.T) {
	service := New()
	params := &pb.RpcAIWritingToolsRequest{
		Mode:        0,
		Language:    0,
		Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
		Endpoint:    "http://localhost:11434/v1/chat/completions",
		Model:       "llama3.2:3b",
		Token:       "",
		Temperature: 0.1,
		Text:        "What is the capital of France?",
	}

	result, err := service.WritingTools(context.Background(), params)
	assert.NoError(t, err)
	assert.NotEmpty(t, result.Answer)
	assert.Contains(t, result.Answer, "The capital of France is Paris.")
}

func TestWritingTools_ValidResponseOpenAI(t *testing.T) {
	err := godotenv.Load()
	assert.NoError(t, err)
	openaiAPIKey := os.Getenv("OPENAI_API_KEY")

	service := New()
	params := &pb.RpcAIWritingToolsRequest{
		Mode:        0,
		Language:    0,
		Provider:    pb.RpcAIWritingToolsRequest_OPENAI,
		Endpoint:    "https://api.openai.com/v1/chat/completions",
		Model:       "gpt-4o-mini",
		Token:       openaiAPIKey,
		Temperature: 0.1,
		Text:        "What is the capital of France?",
	}

	result, err := service.WritingTools(context.Background(), params)
	assert.NoError(t, err)
	assert.NotEmpty(t, result.Answer)
	assert.Contains(t, result.Answer, "The capital of France is Paris.")
}

func TestWritingTools_JSONExtraction(t *testing.T) {
	service := New()
	params := &pb.RpcAIWritingToolsRequest{
		Mode:        6,
		Language:    0,
		Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
		Endpoint:    "http://localhost:11434/v1/chat/completions",
		Model:       "llama3.2:3b",
		Token:       "",
		Temperature: 0.1,
		Text:        "Countries, Capitals\nFrance, Paris\nGermany, Berlin",
	}

	result, err := service.WritingTools(context.Background(), params)
	assert.NoError(t, err)
	assert.Equal(t, "| Country | Capital |\n|----------|--------|\n| France   | Paris  |\n| Germany  | Berlin  |\n", result.Answer)
}
