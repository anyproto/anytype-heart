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

func init() {
	_ = godotenv.Load() // Ensure dotenv is loaded for tests needing API keys
}

func TestOllamaWritingTools(t *testing.T) {
	providerFilter := os.Getenv("TEST_PROVIDER")
	if providerFilter != "" && providerFilter != pb.RpcAIWritingToolsRequest_OLLAMA.String() {
		t.Skipf("Skipping Ollama tests, since TEST_PROVIDER=%s", providerFilter)
	}

	service := New()
	baseParams := &pb.RpcAIWritingToolsRequest{
		Mode:        0,
		Language:    0,
		Provider:    pb.RpcAIWritingToolsRequest_OLLAMA,
		Endpoint:    "http://localhost:11434/v1/chat/completions",
		Model:       "llama3.2:3b",
		Token:       "",
		Temperature: 0,
	}

	t.Run("SupportedLanguage", func(t *testing.T) {
		params := *baseParams
		params.Text = "This is a test."
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
	})

	t.Run("UnsupportedLanguage", func(t *testing.T) {
		params := *baseParams
		params.Text = "Съешь ещё этих мягких французских булок, да выпей же чаю."
		_, err := service.WritingTools(context.Background(), &params)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrUnsupportedLanguage))
	})

	t.Run("InvalidEndpoint", func(t *testing.T) {
		params := *baseParams
		params.Endpoint = "http://invalid-endpoint"
		params.Text = "Can you use an invalid endpoint with Ollama?"
		_, err := service.WritingTools(context.Background(), &params)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrEndpointNotReachable))
	})

	t.Run("InvalidModel", func(t *testing.T) {
		params := *baseParams
		params.Model = "invalid-model"
		params.Text = "Can you use an invalid model with Ollama?"
		_, err := service.WritingTools(context.Background(), &params)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrModelNotFound))
	})

	t.Run("ValidResponse", func(t *testing.T) {
		params := *baseParams
		params.Text = "What is the capital of France?"
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		assert.Contains(t, result.Answer, "Paris")
	})

	t.Run("BulletPoints", func(t *testing.T) {
		params := *baseParams
		params.Mode = 5
		params.Text = "Items to buy: Milk, Eggs, Bread, and Butter. Also, consider Apples if they are on sale."
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		assert.Equal(t, "* Milk\n* Eggs\n* Bread\n* Butter\n* Apples (if on sale)", result.Answer)
	})

	t.Run("JSONExtraction", func(t *testing.T) {
		params := *baseParams
		params.Mode = 6
		params.Text = "Countries, Capitals\nFrance, Paris\nGermany, Berlin"
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		assert.Equal(t,
			"| Country | Capital |\n|----------|---------|\n| France   | Paris   |\n| Germany  | Berlin  |\n",
			result.Answer)
	})
}

func TestOpenAIWritingTools(t *testing.T) {
	providerFilter := os.Getenv("TEST_PROVIDER")
	if providerFilter != "" && providerFilter != pb.RpcAIWritingToolsRequest_OPENAI.String() {
		t.Skipf("Skipping OpenAI tests, since TEST_PROVIDER=%s", providerFilter)
	}

	service := New()
	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	if openaiAPIKey == "" {
		log.Warn("OPENAI_API_KEY not found in environment, using default invalid token")
		openaiAPIKey = "default-invalid-token"
	}

	baseParams := &pb.RpcAIWritingToolsRequest{
		Mode:        0,
		Language:    0,
		Provider:    pb.RpcAIWritingToolsRequest_OPENAI,
		Endpoint:    "https://api.openai.com/v1/chat/completions",
		Model:       "gpt-4o-mini",
		Token:       openaiAPIKey,
		Temperature: 0,
	}

	t.Run("InvalidEndpoint", func(t *testing.T) {
		params := *baseParams
		params.Endpoint = "http://invalid-endpoint"
		params.Text = "Can you use an invalid endpoint with OpenAI?"
		_, err := service.WritingTools(context.Background(), &params)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrEndpointNotReachable))
	})

	t.Run("InvalidModel", func(t *testing.T) {
		params := *baseParams
		params.Model = "invalid-model"
		params.Text = "Can you use an invalid model with OpenAI?"
		_, err := service.WritingTools(context.Background(), &params)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrModelNotFound))
	})

	t.Run("UnauthorizedAccess", func(t *testing.T) {
		params := *baseParams
		params.Token = "invalid-token"
		params.Text = "Can you use an invalid token?"
		_, err := service.WritingTools(context.Background(), &params)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrAuthRequired))
	})

	t.Run("ValidResponse", func(t *testing.T) {
		params := *baseParams
		params.Text = "What is the capital of France?"
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		assert.Contains(t, result.Answer, "Paris")
	})

	t.Run("BulletPoints", func(t *testing.T) {
		params := *baseParams
		params.Mode = 5
		params.Text = "Items to buy: Milk, Eggs, Bread, and Butter. Also, consider Apples if they are on sale."
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		assert.Equal(t, "- Items to buy:\n  - Milk\n  - Eggs\n  - Bread\n  - Butter\n- Consider apples if they are on sale.\n", result.Answer)
	})

	t.Run("JSONExtraction", func(t *testing.T) {
		params := *baseParams
		params.Mode = 6
		params.Text = "Countries, Capitals\nFrance, Paris\nGermany, Berlin"
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		assert.Equal(t,
			"| Country  | Capital |\n|----------|---------|\n| France   | Paris   |\n| Germany  | Berlin  |",
			result.Answer)
	})
}

func TestLMStudioWritingTools(t *testing.T) {
	providerFilter := os.Getenv("TEST_PROVIDER")
	if providerFilter != "" && providerFilter != pb.RpcAIWritingToolsRequest_LMSTUDIO.String() {
		t.Skipf("Skipping LMStudio tests, since TEST_PROVIDER=%s", providerFilter)
	}

	service := New()
	baseParams := &pb.RpcAIWritingToolsRequest{
		Mode:        0,
		Language:    0,
		Provider:    pb.RpcAIWritingToolsRequest_LMSTUDIO,
		Endpoint:    "http://localhost:1234/v1/chat/completions",
		Model:       "llama-3.2-3b-instruct",
		Token:       "",
		Temperature: 0,
	}

	t.Run("InvalidEndpoint", func(t *testing.T) {
		params := *baseParams
		params.Endpoint = "http://invalid-endpoint"
		params.Text = "Can you use an invalid endpoint with LMStudio?"
		_, err := service.WritingTools(context.Background(), &params)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrEndpointNotReachable))
	})

	// TODO: LMStudio doesn't return error for invalid model if one is loaded into memory
	// t.Run("InvalidModel", func(t *testing.T) {
	// 	params := *baseParams
	// 	params.Model = "invalid-model"
	// 	params.Text = "Can you use an invalid model with LMStudio?"
	// 	_, err := service.WritingTools(context.Background(), &params)
	// 	assert.Error(t, err)
	// 	assert.True(t, errors.Is(err, ErrModelNotFound))
	// })

	t.Run("ValidResponse", func(t *testing.T) {
		params := *baseParams
		params.Text = "What is the capital of France?"
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		assert.Contains(t, result.Answer, "Paris")
	})

	t.Run("BulletPoints", func(t *testing.T) {
		params := *baseParams
		params.Mode = 5
		params.Text = "Items to buy: Milk, Eggs, Bread, and Butter. Also, consider Apples if they are on sale."
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		assert.Equal(t, "My Shopping List:\n* Milk\n* Eggs\n* Bread\n* Butter\n* Apples (if on sale)", result.Answer)
	})

	t.Run("JSONExtraction", func(t *testing.T) {
		params := *baseParams
		params.Mode = 6
		params.Text = "Countries, Capitals\nFrance, Paris\nGermany, Berlin"
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		assert.Equal(t,
			"| Country | Capital |\n| --- | --- |\n| France | Paris |\n| Germany | Berlin |",
			result.Answer)
	})
}
