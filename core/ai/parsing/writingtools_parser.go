package parsing

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/pb"
)

// WritingToolsParser is a ResponseParser for WritingTools responses.
type WritingToolsParser struct {
	modeToField  map[int]string
	modeToSchema map[int]func(key string) map[string]interface{}
}

type WritingToolsResponse struct {
	Summary      string `json:"summary,omitempty"`
	Corrected    string `json:"corrected,omitempty"`
	Shortened    string `json:"shortened,omitempty"`
	Expanded     string `json:"expanded,omitempty"`
	Bullet       string `json:"bullet,omitempty"`
	Table        string `json:"table,omitempty"`
	Casual       string `json:"casual,omitempty"`
	Funny        string `json:"funny,omitempty"`
	Confident    string `json:"confident,omitempty"`
	Straight     string `json:"straight,omitempty"`
	Professional string `json:"professional,omitempty"`
	Translation  string `json:"translation,omitempty"`
}

func NewWritingToolsParser() *WritingToolsParser {
	return &WritingToolsParser{
		modeToField: map[int]string{
			int(pb.RpcAIWritingToolsRequest_SUMMARIZE):       "summary",
			int(pb.RpcAIWritingToolsRequest_GRAMMAR):         "corrected",
			int(pb.RpcAIWritingToolsRequest_SHORTEN):         "shortened",
			int(pb.RpcAIWritingToolsRequest_EXPAND):          "expanded",
			int(pb.RpcAIWritingToolsRequest_BULLET):          "bullet",
			int(pb.RpcAIWritingToolsRequest_TABLE):           "table",
			int(pb.RpcAIWritingToolsRequest_CASUAL):          "casual",
			int(pb.RpcAIWritingToolsRequest_FUNNY):           "funny",
			int(pb.RpcAIWritingToolsRequest_CONFIDENT):       "confident",
			int(pb.RpcAIWritingToolsRequest_STRAIGHTFORWARD): "straight",
			int(pb.RpcAIWritingToolsRequest_PROFESSIONAL):    "professional",
			int(pb.RpcAIWritingToolsRequest_TRANSLATE):       "translation",
		},
		modeToSchema: map[int]func(key string) map[string]interface{}{
			int(pb.RpcAIWritingToolsRequest_SUMMARIZE):       SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_GRAMMAR):         SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_SHORTEN):         SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_EXPAND):          SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_BULLET):          SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_TABLE):           SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_CASUAL):          SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_FUNNY):           SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_CONFIDENT):       SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_STRAIGHTFORWARD): SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_PROFESSIONAL):    SingleStringSchema,
			int(pb.RpcAIWritingToolsRequest_TRANSLATE):       SingleStringSchema,
		},
	}
}

func (p *WritingToolsParser) newResponseStruct() interface{} {
	return &WritingToolsResponse{}
}

func (p *WritingToolsParser) ModeToField() map[int]string {
	return p.modeToField
}

func (p *WritingToolsParser) ModeToSchema() map[int]func(key string) map[string]interface{} {
	return p.modeToSchema
}

func (p *WritingToolsParser) ExtractContent(jsonData string, mode int) (ExtractionResult, error) {
	var wtResp WritingToolsResponse
	if err := json.Unmarshal([]byte(jsonData), &wtResp); err != nil {
		return ExtractionResult{}, fmt.Errorf("error parsing JSON: %w %s", err, jsonData)
	}

	fieldName, exists := p.modeToField[mode]
	if !exists {
		return ExtractionResult{}, fmt.Errorf("unknown mode: %d", mode)
	}

	switch fieldName {
	case "summary":
		return ExtractionResult{Raw: wtResp.Summary}, checkEmpty(wtResp.Summary, mode)
	case "corrected":
		return ExtractionResult{Raw: wtResp.Corrected}, checkEmpty(wtResp.Corrected, mode)
	case "shortened":
		return ExtractionResult{Raw: wtResp.Shortened}, checkEmpty(wtResp.Shortened, mode)
	case "expanded":
		return ExtractionResult{Raw: wtResp.Expanded}, checkEmpty(wtResp.Expanded, mode)
	case "bullet":
		return ExtractionResult{Raw: wtResp.Bullet}, checkEmpty(wtResp.Bullet, mode)
	case "table":
		return ExtractionResult{Raw: wtResp.Table}, checkEmpty(wtResp.Table, mode)
	case "casual":
		return ExtractionResult{Raw: wtResp.Casual}, checkEmpty(wtResp.Casual, mode)
	case "funny":
		return ExtractionResult{Raw: wtResp.Funny}, checkEmpty(wtResp.Funny, mode)
	case "confident":
		return ExtractionResult{Raw: wtResp.Confident}, checkEmpty(wtResp.Confident, mode)
	case "straight":
		return ExtractionResult{Raw: wtResp.Straight}, checkEmpty(wtResp.Straight, mode)
	case "professional":
		return ExtractionResult{Raw: wtResp.Professional}, checkEmpty(wtResp.Professional, mode)
	case "translation":
		return ExtractionResult{Raw: wtResp.Translation}, checkEmpty(wtResp.Translation, mode)
	default:
		return ExtractionResult{}, fmt.Errorf("field %s is not recognized", fieldName)
	}
}
