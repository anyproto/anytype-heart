package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/pemistahl/lingua-go"

	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("ai")

var DefaultToken = ""

var (
	ErrUnsupportedLanguage  = errors.New("unsupported input language detected")
	ErrEndpointNotReachable = errors.New("endpoint not reachable")
	ErrModelNotFound        = errors.New("model not found at specified endpoint")
	ErrAuthRequired         = errors.New("api key not provided or invalid for endpoint")
)

const (
	CName = "ai"
)

type AI interface {
	WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (result, error)
	app.ComponentRunnable
}

type AIService struct {
	mu sync.Mutex
}

type APIConfig struct {
	Provider     pb.RpcAIWritingToolsRequestProvider
	Model        string
	Endpoint     string
	AuthRequired bool
	AuthToken    string
}

type PromptConfig struct {
	SystemPrompt string
	UserPrompt   string
	Temperature  float32
	JSONMode     bool
}

func New() AI {
	return &AIService{}
}

func (l *AIService) Init(a *app.App) (err error) {
	return
}

func (l *AIService) Name() (name string) {
	return CName
}

func (l *AIService) Run(_ context.Context) error {
	return nil
}

func (l *AIService) Close(_ context.Context) error {
	return nil
}

type result struct {
	Answer string
}

func (r result) TryClose(objectTTL time.Duration) (bool, error) {
	return true, r.Close()
}

func (r result) Close() error {
	return nil
}

func (l *AIService) WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (result, error) {
	text := strings.ToLower(strings.TrimSpace(params.Text))

	if params.Provider == pb.RpcAIWritingToolsRequest_OLLAMA {
		languages := []lingua.Language{lingua.English, lingua.Spanish, lingua.French, lingua.German, lingua.Italian, lingua.Portuguese, lingua.Hindi, lingua.Thai}
		detector := lingua.NewLanguageDetectorBuilder().
			FromLanguages(languages...).
			WithLowAccuracyMode().
			Build()

		if language, exists := detector.DetectLanguageOf(text); !exists {
			return result{}, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, language)
		}
	}

	configChat := APIConfig{
		Provider:     params.Provider,
		Endpoint:     params.Endpoint,
		Model:        params.Model,
		AuthRequired: params.Provider != pb.RpcAIWritingToolsRequest_OLLAMA,
		AuthToken:    params.Token,
	}

	configPrompt := PromptConfig{
		SystemPrompt: systemPrompts[params.Mode],
		UserPrompt:   fmt.Sprintf(userPrompts[params.Mode], text),
		Temperature:  params.Temperature,
		JSONMode:     params.Mode != 1,
	}

	answerChunks, err := chat(configChat, configPrompt)
	if err != nil {
		return result{}, err
	}

	var answerBuilder strings.Builder
	for _, chunk := range *answerChunks {
		for _, choice := range chunk.Choices {
			answerBuilder.WriteString(choice.Delta.Content)
		}
	}

	return result{Answer: answerBuilder.String()}, nil
}

func init() {
	if DefaultToken == "" {
		DefaultToken = loadenv.Get("OPENAI_API_KEY")
	}
}
