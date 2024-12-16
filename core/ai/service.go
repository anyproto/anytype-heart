package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/pemistahl/lingua-go"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	CName       = "ai"
	httpTimeout = 30 * time.Second
)

var log = logging.Logger("ai")

var (
	ErrRateLimitExceeded    = errors.New("rate limit exceeded")
	ErrUnsupportedLanguage  = errors.New("unsupported input language detected")
	ErrEndpointNotReachable = errors.New("endpoint not reachable")
	ErrModelNotFound        = errors.New("model not found at specified endpoint")
	ErrAuthRequired         = errors.New("api key not provided or invalid for endpoint")
)

type AI interface {
	WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (Result, error)
	Autofill(ctx context.Context, params *pb.RpcAIAutofillRequest) (Result, error)
	app.ComponentRunnable
}

type AIService struct {
	mu                       sync.Mutex
	apiConfig                *APIConfig
	writingToolsPromptConfig *WritingToolsPromptConfig
	autofillPromptConfig     *AutofillPromptConfig
	httpClient               HttpClient
}

type APIConfig struct {
	Provider     pb.RpcAIProvider
	Model        string
	Endpoint     string
	AuthRequired bool
	AuthToken    string
}

type WritingToolsPromptConfig struct {
	Mode         pb.RpcAIWritingToolsRequestWritingMode
	SystemPrompt string
	UserPrompt   string
	Temperature  float32
	JSONMode     bool
}

type AutofillPromptConfig struct {
	Mode         pb.RpcAIAutofillRequestAutofillMode
	SystemPrompt string
	UserPrompt   string
	Temperature  float32
	JSONMode     bool
	Options      []string
	Context      []string
}

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Result struct {
	Answer  string
	Choices []string
}

func New() AI {
	return &AIService{
		httpClient: &http.Client{Timeout: httpTimeout},
	}
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
	if !(params.Config.Provider == pb.RpcAI_OPENAI) {
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

	ai.setAPIConfig(params.Config)
	ai.writingToolsPromptConfig = &WritingToolsPromptConfig{
		Mode:         params.Mode,
		SystemPrompt: writingToolsSystemPrompts[params.Mode],
		UserPrompt:   fmt.Sprintf(writingToolsUserPrompts[params.Mode], text),
		Temperature:  params.Config.Temperature,
		JSONMode:     params.Mode != 0, // use json mode for all modes except default
	}

	answer, err := ai.chat(ctx)
	if err != nil {
		return Result{}, err
	}

	// extract answer value from json response, except for default mode
	if params.Mode != 0 {
		extractedAnswer, err := ai.extractAnswerByMode(answer, "writingTools")
		if err != nil {
			return Result{}, err
		}

		// fix lmstudio newline issue for table responses
		extractedAnswer = strings.ReplaceAll(extractedAnswer, "\\\\n", "\n")
		return Result{Answer: extractedAnswer}, nil
	}

	return Result{Answer: answer}, nil
}

func (ai *AIService) Autofill(ctx context.Context, params *pb.RpcAIAutofillRequest) (Result, error) {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	ai.setAPIConfig(params.Config)
	ai.autofillPromptConfig = &AutofillPromptConfig{
		Mode:         params.Mode,
		SystemPrompt: autofillSystemPrompts[params.Mode],
		// TODO: create prompt with options and context
		UserPrompt:  fmt.Sprintf(autofillUserPrompts[params.Mode], params.Options, params.Context),
		Temperature: params.Config.Temperature,
		JSONMode:    true,
		Options:     params.Options,
		Context:     params.Context,
	}

	answer, err := ai.chat(ctx)
	if err != nil {
		return Result{}, err
	}

	return Result{Choices: []string{"not implemented yet", answer}}, nil
}

func (ai *AIService) setAPIConfig(params *pb.RpcAIProviderConfig) {
	ai.apiConfig = &APIConfig{
		Provider:     params.Provider,
		Endpoint:     params.Endpoint,
		Model:        params.Model,
		AuthRequired: params.Provider == pb.RpcAI_OPENAI,
		AuthToken:    params.Token,
	}
}
