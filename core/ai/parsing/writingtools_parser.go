package parsing

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pb"
)

// WritingToolsParser is a ResponseParser for WritingTools responses.
// It knows how to extract the correct field from the WritingToolsResponse based on the mode.
type WritingToolsParser struct {
	modeToField  map[int]string
	modeToSchema map[int]func(key string) map[string]interface{}
}

// WritingToolsResponse represents the structure of the content response for different writing tool modes.
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

// NewWritingToolsParser returns a new WritingToolsParser instance.
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

// NewResponseStruct returns a new WritingToolsResponse instance.
func (p *WritingToolsParser) NewResponseStruct() interface{} {
	return &WritingToolsResponse{}
}

// ModeToField returns the modeToField map.
func (p *WritingToolsParser) ModeToField() map[int]string {
	return p.modeToField
}

// ModeToSchema returns the modeToSchema map.
func (p *WritingToolsParser) ModeToSchema() map[int]func(key string) map[string]interface{} {
	return p.modeToSchema
}

// ExtractContent extracts the relevant field based on mode.
func (p *WritingToolsParser) ExtractContent(mode int, response interface{}) (string, error) {
	wtResp, ok := response.(*WritingToolsResponse)
	if !ok {
		return "", fmt.Errorf("invalid response type, expected *WritingToolsResponse")
	}

	fieldName, exists := p.modeToField[mode]
	if !exists {
		return "", fmt.Errorf("unknown mode: %d", mode)
	}

	// Extract the correct field based on fieldName.
	switch fieldName {
	case "summary":
		return wtResp.Summary, CheckEmpty(wtResp.Summary, mode)
	case "corrected":
		return wtResp.Corrected, CheckEmpty(wtResp.Corrected, mode)
	case "shortened":
		return wtResp.Shortened, CheckEmpty(wtResp.Shortened, mode)
	case "expanded":
		return wtResp.Expanded, CheckEmpty(wtResp.Expanded, mode)
	case "bullet":
		return wtResp.Bullet, CheckEmpty(wtResp.Bullet, mode)
	case "table":
		return wtResp.Table, CheckEmpty(wtResp.Table, mode)
	case "casual":
		return wtResp.Casual, CheckEmpty(wtResp.Casual, mode)
	case "funny":
		return wtResp.Funny, CheckEmpty(wtResp.Funny, mode)
	case "confident":
		return wtResp.Confident, CheckEmpty(wtResp.Confident, mode)
	case "straight":
		return wtResp.Straight, CheckEmpty(wtResp.Straight, mode)
	case "professional":
		return wtResp.Professional, CheckEmpty(wtResp.Professional, mode)
	case "translation":
		return wtResp.Translation, CheckEmpty(wtResp.Translation, mode)
	default:
		return "", fmt.Errorf("field %s is not recognized", fieldName)
	}
}
