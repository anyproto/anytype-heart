package ai

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
)

func init() {
	_ = godotenv.Load()
}

type modelTestConfig struct {
	modelName            string
	expectValidResponse  string // Expected exact answer for the "ValidResponse" test scenario
	expectBulletPoints   string // Expected exact answer for the "BulletPoints" test scenario
	expectJSONExtraction string // Expected exact answer for the "JSONExtraction" test scenario
}

type providerTestConfig struct {
	name                   string
	provider               pb.RpcAIProvider
	writingToolsBaseParams pb.RpcAIWritingToolsRequest
	autofillBaseParams     pb.RpcAIAutofillRequest
	skipInputLanguageTest  bool // check supported languages for llama models
	skipAuthTest           bool // server side providers require auth
	models                 []modelTestConfig
}

func copyProviderConfig(original *pb.RpcAIProviderConfig) *pb.RpcAIProviderConfig {
	if original == nil {
		return nil
	}
	return proto.Clone(original).(*pb.RpcAIProviderConfig)
}

// WritingTools
// ***
func TestWritingTools(t *testing.T) {
	providerFilter := os.Getenv("TEST_PROVIDER")

	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	if openaiAPIKey == "" {
		log.Warn("OPENAI_API_KEY not found, using default invalid token")
		openaiAPIKey = "default-invalid-token"
	}

	testConfigs := []providerTestConfig{
		{
			name:     "Ollama",
			provider: pb.RpcAI_OLLAMA,
			writingToolsBaseParams: pb.RpcAIWritingToolsRequest{
				Config: &pb.RpcAIProviderConfig{
					Provider:    pb.RpcAI_OLLAMA,
					Endpoint:    "http://localhost:11434/v1/chat/completions",
					Model:       "llama3.2:3b",
					Token:       "",
					Temperature: 0,
				},
				Mode:     0,
				Language: 0,
			},
			skipInputLanguageTest: false,
			skipAuthTest:          true,
			models: []modelTestConfig{
				{
					modelName:            "llama3.2:3b",
					expectValidResponse:  "Paris",
					expectBulletPoints:   "* Milk\n* Eggs\n* Bread\n* Butter\n* Apples (consider if on sale)",
					expectJSONExtraction: "| Country | Capital |\n|----------|---------|\n| France   | Paris   |\n| Germany  | Berlin  |",
				},
				{
					modelName:            "llama3.1:8b",
					expectValidResponse:  "Paris",
					expectBulletPoints:   "• items to buy:\n\n  • milk\n\n  • eggs\n\n  • bread\n\n  • butter\n\nConsider apples if on sale.",
					expectJSONExtraction: "| Country | Capital |\n| --- | --- |\n| France | Paris |\n| Germany | Berlin |",
				},
			},
		},
		{
			name:     "OpenAI",
			provider: pb.RpcAI_OPENAI,
			writingToolsBaseParams: pb.RpcAIWritingToolsRequest{
				Config: &pb.RpcAIProviderConfig{
					Provider:    pb.RpcAI_OPENAI,
					Endpoint:    "https://api.openai.com/v1/chat/completions",
					Model:       "gpt-4o-mini",
					Token:       openaiAPIKey,
					Temperature: 0,
				},
				Mode:     0,
				Language: 0,
			},
			skipInputLanguageTest: true,
			skipAuthTest:          false,
			models: []modelTestConfig{
				{
					modelName:            "gpt-4o-mini",
					expectValidResponse:  "Paris",
					expectBulletPoints:   "- Items to buy:\n  - Milk\n  - Eggs\n  - Bread\n  - Butter\n- Consider apples if on sale.\n",
					expectJSONExtraction: "| Country  | Capital |\n|----------|---------|\n| France   | Paris   |\n| Germany  | Berlin  |",
				},
				// {
				// 	modelName:            "gpt-4o",
				// 	expectValidResponse:  "Paris",
				// 	expectBulletPoints:   "- Items to buy:\n  - Milk\n  - Eggs\n  - Bread\n  - Butter\n- Consider buying apples if they are on sale.\n",
				// 	expectJSONExtraction: "| Country | Capital |\n|---------|---------|\n| France  | Paris   |\n| Germany | Berlin  |",
				// },
			},
		},
		{
			name:     "LMStudio",
			provider: pb.RpcAI_LMSTUDIO,
			writingToolsBaseParams: pb.RpcAIWritingToolsRequest{
				Config: &pb.RpcAIProviderConfig{
					Provider:    pb.RpcAI_LMSTUDIO,
					Endpoint:    "http://localhost:1234/v1/chat/completions",
					Model:       "llama-3.2-3b-instruct",
					Token:       "",
					Temperature: 0,
				},
				Mode:     0,
				Language: 0,
			},
			skipInputLanguageTest: false,
			skipAuthTest:          true,
			models: []modelTestConfig{
				{
					modelName:            "llama-3.2-3b-instruct",
					expectValidResponse:  "Paris",
					expectBulletPoints:   "My Shopping List:\\\\ items to buy:\\\\\\  - milk\\\\\\  - eggs\\\\\\  - bread\\\\\\  - butter\\\\\\  consider apples if on sale.",
					expectJSONExtraction: "| Country | Capital |\n| --- | --- |\n| france | paris |\n| germany | berlin |",
				},
				// {
				// 	modelName:            "meta-llama-3.1-8b-instruct",
				// 	expectValidResponse:  "Paris",
				// 	expectBulletPoints:   "- items to buy: milk, eggs, bread, butter.\"- consider apples if on sale. \"- check prices before buying.",
				// 	expectJSONExtraction: ">\\\\| Country  | Capital |\\\\\\hline\\\nFrance     | Paris   \\\\| Germany  | Berlin   \\\\|",
				// },
			},
		},
		{
			name:     "Llama.cpp",
			provider: pb.RpcAI_LLAMACPP,
			writingToolsBaseParams: pb.RpcAIWritingToolsRequest{
				Config: &pb.RpcAIProviderConfig{
					Provider:    pb.RpcAI_LLAMACPP,
					Endpoint:    "http://localhost:8080/v1/chat/completions",
					Model:       "Llama-3.2-3B-Instruct-Q6_K_L",
					Token:       "",
					Temperature: 0,
				},
				Mode:     0,
				Language: 0,
			},
			skipInputLanguageTest: false,
			skipAuthTest:          true,
			models: []modelTestConfig{
				{
					modelName:            "Llama-3.2-3B-Instruct",
					expectValidResponse:  "Paris",
					expectBulletPoints:   "* Milk\n* Eggs\n* Bread\n* Butter\n* Apples (if on sale)",
					expectJSONExtraction: "| Country | Capital |\n|----------|--------|\n| France   | Paris  |\n| Germany  | Berlin |\n",
				},
			},
		},
	}

	for _, cfg := range testConfigs {
		cfg := cfg
		if providerFilter != "" && providerFilter != cfg.provider.String() {
			continue
		}

		t.Run(cfg.name, func(t *testing.T) {
			service := New()

			for _, modelCfg := range cfg.models {
				modelCfg := modelCfg
				t.Run(modelCfg.modelName, func(t *testing.T) {
					runWritingToolsTests(t, service, cfg, modelCfg)
				})
			}
		})
	}
}

func runWritingToolsTests(t *testing.T, service AI, cfg providerTestConfig, modelCfg modelTestConfig) {
	t.Run("InvalidEndpoint", func(t *testing.T) {
		params := cfg.writingToolsBaseParams
		params.Config = copyProviderConfig(params.Config)
		params.Config.Model = modelCfg.modelName
		params.Config.Endpoint = "http://invalid-endpoint"
		params.Text = "Test invalid endpoint"
		_, err := service.WritingTools(context.Background(), &params)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrEndpointNotReachable))
	})

	if !cfg.skipInputLanguageTest {
		t.Run("UnsupportedLanguage", func(t *testing.T) {
			params := cfg.writingToolsBaseParams
			params.Config.Model = modelCfg.modelName
			params.Text = "Съешь ещё этих мягких французских булок"
			_, err := service.WritingTools(context.Background(), &params)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, ErrUnsupportedLanguage))
		})
	}

	t.Run("InvalidModel", func(t *testing.T) {
		params := cfg.writingToolsBaseParams
		params.Config.Model = "invalid-model"
		params.Text = "Test invalid model"
		_, err := service.WritingTools(context.Background(), &params)
		if err != nil {
			assert.True(t, errors.Is(err, ErrModelNotFound))
		} else {
			// LMSStudio doesn't return error for invalid model; instead falls back to model loaded into memory
			t.Skipf("%s does not error out for invalid model %s", cfg.name, params.Config.Model)
		}
	})

	if !cfg.skipAuthTest {
		t.Run("UnauthorizedAccess", func(t *testing.T) {
			params := cfg.writingToolsBaseParams
			params.Config = copyProviderConfig(params.Config)
			params.Config.Model = modelCfg.modelName
			params.Config.Token = "invalid-token"
			params.Text = "Test unauthorized access"
			_, err := service.WritingTools(context.Background(), &params)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, ErrAuthRequired))
		})
	}

	t.Run("ValidResponse", func(t *testing.T) {
		params := cfg.writingToolsBaseParams
		params.Config.Model = modelCfg.modelName
		params.Text = "What is the capital of France?"
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		if modelCfg.expectValidResponse != "" {
			assert.Contains(t, result.Answer, modelCfg.expectValidResponse)
		} else {
			t.Errorf("Expected valid response not provided for %s", modelCfg.modelName)
		}
	})

	t.Run("BulletPoints", func(t *testing.T) {
		params := cfg.writingToolsBaseParams
		params.Config.Model = modelCfg.modelName
		params.Mode = 5
		params.Text = "Items to buy: Milk, Eggs, Bread, Butter. Consider Apples if on sale."
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		if modelCfg.expectBulletPoints != "" {
			assert.Equal(t, modelCfg.expectBulletPoints, result.Answer)
		} else {
			t.Errorf("Expected bullet points not provided for %s", modelCfg.modelName)
		}
	})

	t.Run("JSONExtraction", func(t *testing.T) {
		params := cfg.writingToolsBaseParams
		params.Config.Model = modelCfg.modelName
		params.Mode = 6
		params.Text = "Countries, Capitals\nFrance, Paris\nGermany, Berlin"
		result, err := service.WritingTools(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Answer)
		if modelCfg.expectJSONExtraction != "" {
			assert.Equal(t, modelCfg.expectJSONExtraction, result.Answer)
		} else {
			t.Errorf("Expected JSON extraction not provided for %s", modelCfg.modelName)
		}
	})
}

// Autofill
// ***
func TestAutofill(t *testing.T) {
	providerFilter := os.Getenv("TEST_PROVIDER")

	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	if openaiAPIKey == "" {
		log.Warn("OPENAI_API_KEY not found, using default invalid token")
		openaiAPIKey = "default-invalid-token"
	}

	testConfigs := []providerTestConfig{
		{
			name:     "Ollama",
			provider: pb.RpcAI_OLLAMA,
			autofillBaseParams: pb.RpcAIAutofillRequest{
				Config: &pb.RpcAIProviderConfig{
					Provider:    pb.RpcAI_OLLAMA,
					Endpoint:    "http://localhost:11434/v1/chat/completions",
					Model:       "llama3.2:3b",
					Token:       "",
					Temperature: 0,
				},
				Mode:    0,
				Options: []string{"book", "movie", "song"},
				Context: []string{"I am reading a"},
			},
			skipInputLanguageTest: false,
			skipAuthTest:          true,
			models: []modelTestConfig{
				{
					modelName: "llama3.2:3b",
					// TODO: refactor types
				},
			},
		},
		{
			name:     "OpenAI",
			provider: pb.RpcAI_OPENAI,
			autofillBaseParams: pb.RpcAIAutofillRequest{
				Config: &pb.RpcAIProviderConfig{
					Provider:    pb.RpcAI_OPENAI,
					Endpoint:    "https://api.openai.com/v1/chat/completions",
					Model:       "gpt-4o-mini",
					Token:       openaiAPIKey,
					Temperature: 0,
				},
				Mode:    0,
				Options: []string{"book", "movie", "song"},
				Context: []string{"I am reading a"},
			},
			skipInputLanguageTest: true,
			skipAuthTest:          false,
			models: []modelTestConfig{
				{
					modelName: "gpt-4o-mini",
					// TODO: refactor types
				},
			},
		},
	}

	for _, cfg := range testConfigs {
		cfg := cfg
		if providerFilter != "" && providerFilter != cfg.provider.String() {
			continue
		}

		t.Run(cfg.name, func(t *testing.T) {
			service := New()

			for _, modelCfg := range cfg.models {
				modelCfg := modelCfg
				t.Run(modelCfg.modelName, func(t *testing.T) {
					runAutofillTests(t, service, cfg, modelCfg)
				})
			}
		})
	}
}

func runAutofillTests(t *testing.T, service AI, cfg providerTestConfig, modelCfg modelTestConfig) {
	t.Run("tag suggest book", func(t *testing.T) {
		params := cfg.autofillBaseParams
		params.Config.Model = modelCfg.modelName
		params.Context = []string{"I am reading a"}
		result, err := service.Autofill(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Choices)
		assert.Equal(t, 1, len(result.Choices))
		assert.Equal(t, "book", result.Choices[0])
	})

	t.Run("tag suggest movie", func(t *testing.T) {
		params := cfg.autofillBaseParams
		params.Config.Model = modelCfg.modelName
		params.Context = []string{"Titanic was published in 1997."}
		result, err := service.Autofill(context.Background(), &params)
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Choices)
		assert.Equal(t, 1, len(result.Choices))
		assert.Equal(t, "movie", result.Choices[0])
	})
}
