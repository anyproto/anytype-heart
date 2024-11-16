package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/pemistahl/lingua-go"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	CName = "ai"
)

var log = logging.Logger("ai")

var (
	ErrUnsupportedLanguage  = errors.New("unsupported input language detected")
	ErrEndpointNotReachable = errors.New("endpoint not reachable")
	ErrModelNotFound        = errors.New("model not found at specified endpoint")
	ErrAuthRequired         = errors.New("api key not provided or invalid for endpoint")
)

type AI interface {
	WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (Result, error)
	app.ComponentRunnable
}

type AIService struct {
	apiConfig    APIConfig
	promptConfig PromptConfig
	mu           sync.Mutex
}

type APIConfig struct {
	Provider     pb.RpcAIWritingToolsRequestProvider
	Model        string
	Endpoint     string
	AuthRequired bool
	AuthToken    string
}

type PromptConfig struct {
	Mode         pb.RpcAIWritingToolsRequestMode
	SystemPrompt string
	UserPrompt   string
	Temperature  float32
	JSONMode     bool
}

type Result struct {
	Answer string
}

func New() AI {
	return &AIService{}
}

func (ai *AIService) Init(a *app.App) (err error) {
	return nil
}

func (ai *AIService) Name() (name string) {
	return CName
}

func (ai *AIService) Run(_ context.Context) (err error) {
	return nil
}

func (ai *AIService) Close(_ context.Context) (err error) {
	return nil
}

func (ai *AIService) WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (Result, error) {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	log.Infof("received request with text: %s", strings.ReplaceAll(params.Text, "\n", "\\n"))
	text := strings.ToLower(strings.TrimSpace(params.Text))

	// check supported languages for llama models
	if params.Provider == pb.RpcAIWritingToolsRequest_OLLAMA {
		languages := []lingua.Language{lingua.English, lingua.Spanish, lingua.French, lingua.German, lingua.Italian, lingua.Portuguese, lingua.Hindi, lingua.Thai}
		detector := lingua.NewLanguageDetectorBuilder().
			FromLanguages(languages...).
			WithLowAccuracyMode().
			Build()

		if language, exists := detector.DetectLanguageOf(text); !exists {
			log.Errorf("unsupported language detected: %s", language)
			return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, language)
		}
	}

	ai.apiConfig = APIConfig{
		Provider:     params.Provider,
		Endpoint:     params.Endpoint,
		Model:        params.Model,
		AuthRequired: params.Provider != pb.RpcAIWritingToolsRequest_OLLAMA,
		AuthToken:    params.Token,
	}

	ai.promptConfig = PromptConfig{
		Mode:         params.Mode,
		SystemPrompt: systemPrompts[params.Mode],
		UserPrompt:   fmt.Sprintf(userPrompts[params.Mode], text),
		Temperature:  params.Temperature,
		JSONMode:     params.Mode != 0, // use json mode for all modes except default
	}

	answer, err := ai.chat(context.Background())
	if err != nil {
		return Result{}, err
	}

	// extract answer value from json response, except for default mode
	if params.Mode != 0 {
		extractedAnswer, err := ai.extractAnswerByMode(answer)
		if err != nil {
			return Result{}, err
		}

		return Result{Answer: extractedAnswer}, nil
	}

	return Result{Answer: answer}, nil
}
