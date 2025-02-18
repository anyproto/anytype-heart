package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/pemistahl/lingua-go"

	"github.com/anyproto/anytype-heart/core/ai/parsing"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	CName       = "ai"
	httpTimeout = 30 * time.Second
)

var log = logging.Logger(CName)

var (
	ErrRateLimitExceeded    = errors.New("rate limit exceeded")
	ErrUnsupportedLanguage  = errors.New("unsupported input language detected")
	ErrEndpointNotReachable = errors.New("endpoint not reachable")
	ErrModelNotFound        = errors.New("model not found at specified endpoint")
	ErrAuthRequired         = errors.New("api key not provided or invalid for endpoint")
)

type AI interface {
	WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (WritingToolsResult, error)
	Autofill(ctx context.Context, params *pb.RpcAIAutofillRequest) (AutofillResult, error)
	WebsiteProcess(ctx context.Context, params *pb.RpcAIWebsiteProcessRequest) (*WebsiteProcessResult, error)
	app.ComponentRunnable
}

type AIService struct {
	mu             sync.Mutex
	apiConfig      *APIConfig
	promptConfig   *PromptConfig
	httpClient     HttpClient
	responseParser parsing.ResponseParser
}

type APIConfig struct {
	Provider     pb.RpcAIProvider
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

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type WritingToolsResult struct {
	Answer string
}

type AutofillResult struct {
	Choices []string
}

type WebsiteProcessResult struct {
	// Type            string            // "recipe", "hotel", or "book"
	// Relations       map[string]string // e.g. {"portions": "2", "prep_time": "40 minutes", ...}
	// MarkdownSummary string            // e.g. "## Pasta with tomato sauce and basil.\n A classic Italian dish ..."
	ObjectId string
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

func (ai *AIService) WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (WritingToolsResult, error) {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	log.Infof("received writing tools request with text: %s", strings.ReplaceAll(params.Text, "\n", "\\n"))
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
			return WritingToolsResult{}, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, language)
		}
	}

	ai.setAPIConfig(params.Config)
	ai.promptConfig = &PromptConfig{
		SystemPrompt: writingToolsPrompts[params.Mode].System,
		UserPrompt:   fmt.Sprintf(writingToolsPrompts[params.Mode].User, text),
		Temperature:  params.Config.Temperature,
		JSONMode:     params.Mode != 0, // use json mode for all modes except default
	}
	ai.responseParser = parsing.NewWritingToolsParser()

	answer, err := ai.chat(ctx, int(params.Mode))
	if err != nil {
		return WritingToolsResult{}, err
	}

	// extract answer value from json response, except for default mode
	if params.Mode != 0 {
		extractedAnswer, err := ai.extractAnswerByMode(answer, int(params.Mode))
		if err != nil {
			return WritingToolsResult{}, err
		}

		// fix lmstudio newline issue for table responses
		extractedAnswer = strings.ReplaceAll(extractedAnswer, "\\\\n", "\n")
		return WritingToolsResult{Answer: extractedAnswer}, nil
	}

	return WritingToolsResult{Answer: answer}, nil
}

func (ai *AIService) Autofill(ctx context.Context, params *pb.RpcAIAutofillRequest) (AutofillResult, error) {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	optionsStr := strings.Join(params.Options, ", ")
	contextStr := strings.Join(params.Context, " ")
	log.Infof("received autofill request with options: %s and context: %s", params.Options, params.Context)

	ai.setAPIConfig(params.Config)
	ai.promptConfig = &PromptConfig{
		SystemPrompt: autofillPrompts[params.Mode].System,
		// TODO: create prompt with options and context
		UserPrompt:  fmt.Sprintf(autofillPrompts[params.Mode].User, optionsStr, contextStr),
		Temperature: params.Config.Temperature,
		JSONMode:    true,
	}
	ai.responseParser = parsing.NewAutofillParser()

	answer, err := ai.chat(ctx, int(params.Mode))
	if err != nil {
		return AutofillResult{}, err
	}

	extractedAnswer, err := ai.extractAnswerByMode(answer, int(params.Mode))
	if err != nil {
		return AutofillResult{}, err
	}

	return AutofillResult{Choices: []string{extractedAnswer}}, nil
}

// WebsiteProcess fetches a URL, classifies it, and extracts relations and a summary.
func (ai *AIService) WebsiteProcess(ctx context.Context, params *pb.RpcAIWebsiteProcessRequest) (*WebsiteProcessResult, error) {
	content, err := FetchAndExtract(params.Url)
	if err != nil {
		return nil, err
	}

	ai.setAPIConfig(params.Config)
	websiteType, err := ai.ClassifyWebsiteContent(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("could not classify website content: %w", err)
	}

	prompts, ok := websiteExtractionPrompts[websiteType]
	if !ok {
		return nil, fmt.Errorf("no extraction prompts for website type: %s", websiteType)
	}

	relationPrompt := fmt.Sprintf(prompts.RelationPrompt, content)
	summaryPrompt := fmt.Sprintf(prompts.SummaryPrompt, content)

	var (
		relationsResult map[string]string
		summaryResult   string
		relErr, sumErr  error
	)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		pc := &PromptConfig{
			SystemPrompt: "",
			UserPrompt:   relationPrompt,
			Temperature:  0,
			JSONMode:     true,
		}
		ai.responseParser = parsing.NewWebsiteProcessParser()

		answer, err := ai.newChat(ctx, 1, pc, ai.responseParser)
		if err != nil {
			relErr = err
			return
		}
		extractedAnswer, err := ai.extractAnswerByMode(answer, 1)

		var relations map[string]string
		if err := json.Unmarshal([]byte(extractedAnswer), &relations); err != nil {
			relErr = err
			return
		}
		relationsResult = relations
	}()

	go func() {
		defer wg.Done()
		pc := &PromptConfig{
			SystemPrompt: "",
			UserPrompt:   summaryPrompt,
			Temperature:  0.2,
			JSONMode:     false,
		}
		ai.responseParser = parsing.NewWebsiteProcessParser()
		answer, err := ai.newChat(ctx, 2, pc, ai.responseParser)
		if err != nil {
			sumErr = err
			return
		}
		summaryResult = answer
	}()

	wg.Wait()
	if relErr != nil {
		return nil, fmt.Errorf("relation extraction failed: %w", relErr)
	}
	if sumErr != nil {
		return nil, fmt.Errorf("summary extraction failed: %w", sumErr)
	}

	log.Infof("website processed successfully, type: %s", websiteType)
	log.Debugf("relations: %v", relationsResult)
	log.Debugf("summary: %s", summaryResult)

	return &WebsiteProcessResult{
		// Type:            websiteType,
		// Relations:       relationsResult,
		// MarkdownSummary: summaryResult,
		// TODO: create object with extracted data and return its Id
		ObjectId: "123",
	}, nil
}

func (ai *AIService) ClassifyWebsiteContent(ctx context.Context, content string) (string, error) {
	systemPrompt := `You are a classification assistant. 
Your task is to classify text into one of the following categories: "recipe", "hotel", or "book".
Return ONLY the category name. Do NOT add explanations, punctuation, or extra words.
If uncertain, answer with best possible guess. If none apply, answer with "none".`
	userPrompt := fmt.Sprintf(`Classify the following content into one of the categories: "recipe", "hotel", or "book".
Answer with ONLY one of these words, nothing else.

Content:
---
%s
---
`, content[:min(len(content), 1000)])
	ai.promptConfig = &PromptConfig{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.2,
		JSONMode:     false,
	}

	answer, err := ai.chat(ctx, 0)
	if err != nil {
		return "", err
	}

	classification := strings.ToLower(strings.TrimSpace(answer))
	switch classification {
	case "recipe", "hotel", "book":
		return classification, nil
	default:
		return "", fmt.Errorf("invalid classification: %s", classification)
	}
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
