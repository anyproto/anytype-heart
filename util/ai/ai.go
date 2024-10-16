package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"

	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("ai")

var DefaultToken = ""

const (
	CName         = "ai"
	cacheTTL      = time.Minute * 10
	cacheGCPeriod = time.Minute * 5
)

type WritingToolsParams struct {
	Mode     pb.RpcAIWritingToolsRequestMode
	Language pb.RpcAIWritingToolsRequestLanguage
	Text     string
	Endpoint string
	Token    string
}

type AI interface {
	WritingTools(ctx context.Context, mode pb.RpcAIWritingToolsRequestMode, text string, endpoint string, language pb.RpcAIWritingToolsRequestLanguage) (result, error)
	// TODO: functions

	app.ComponentRunnable
}

type AIService struct {
	mu    sync.Mutex
	cache ocache.OCache
}

func New() AI {
	return &AIService{}
}

func (l *AIService) Init(a *app.App) (err error) {
	l.cache = ocache.New(l.writingTools, ocache.WithTTL(cacheTTL), ocache.WithGCPeriod(cacheGCPeriod))
	return
}

func (l *AIService) Name() (name string) {
	return CName
}

func (l *AIService) Run(_ context.Context) error {
	return nil
}

func (l *AIService) Close(_ context.Context) error {
	return l.cache.Close()
}

type result struct {
	Text string
	// TODO: fields
}

func (r result) TryClose(objectTTL time.Duration) (bool, error) {
	return true, r.Close()
}

func (r result) Close() error {
	return nil
}

func (l *AIService) WritingTools(ctx context.Context, mode pb.RpcAIWritingToolsRequestMode, text string, endpoint string, language pb.RpcAIWritingToolsRequestLanguage) (result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return result{}, fmt.Errorf("empty text")
	}
	v, err := l.cache.Get(ctx, fmt.Sprintf("%s-%s-%s-%s", mode, text, endpoint, language))
	if err != nil {
		return result{}, err
	}

	if r, ok := v.(result); ok {
		return r, nil
	} else {
		panic("invalid cache value")
	}
}

// TODO: fix signature
func (l *AIService) writingTools(ctx context.Context, query string) (ocache.Object, error) {
	text := strings.ToLower(strings.TrimSpace(query))

	configChat := APIConfig{
		Provider:       ProviderOllama,
		Endpoint:       ollamaEndpointChat,
		EndpointModels: ollamaEndpointModels,
		Model:          ollamaDefaultModelChat,

		AuthRequired: false,
		AuthToken:    "",
	}

	// systemPrompt := systemPrompts[params.Mode]
	// userPrompt := fmt.Sprintf(userPrompts[params.Mode], text)
	configPrompt := PromptConfig{
		SystemPrompt: systemPrompts[2],
		UserPrompt:   fmt.Sprintf(userPrompts[2], text),
		Temperature:  0.1,
		JSONMode:     true,
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

	return result{Text: answerBuilder.String()}, nil
}

func init() {
	if DefaultToken == "" {
		DefaultToken = loadenv.Get("OPENAI_API_KEY")
	}
}
