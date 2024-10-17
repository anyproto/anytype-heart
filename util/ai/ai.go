package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype/config/loadenv"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("ai")

var DefaultToken = ""

const (
	CName = "ai"
)

type AI interface {
	WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (result, error)
	// TODO: functions

	app.ComponentRunnable
}

type AIService struct {
	mu sync.Mutex
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
	Text string
	// TODO: fields
}

func (r result) TryClose(objectTTL time.Duration) (bool, error) {
	return true, r.Close()
}

func (r result) Close() error {
	return nil
}

func (l *AIService) WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (result, error) {
	text := strings.ToLower(strings.TrimSpace(params.Text))

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
