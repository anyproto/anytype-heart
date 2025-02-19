package parsing

import (
	"encoding/json"
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
func (p *WritingToolsParser) ExtractContent(jsonData string, mode int) (ParsedResult, error) {
	respStruct := p.NewResponseStruct()

	err := json.Unmarshal([]byte(jsonData), &respStruct)
	if err != nil {
		return ParsedResult{}, fmt.Errorf("error parsing JSON: %w %s", err, jsonData)
	}

	wtResp, ok := respStruct.(*WritingToolsResponse)
	if !ok {
		return ParsedResult{}, fmt.Errorf("invalid response type, expected *WritingToolsResponse")
	}

	fieldName, exists := p.modeToField[mode]
	if !exists {
		return ParsedResult{}, fmt.Errorf("unknown mode: %d", mode)
	}

	// Extract the correct field based on fieldName.
	switch fieldName {
	case "summary":
		return ParsedResult{Raw: wtResp.Summary}, CheckEmpty(wtResp.Summary, mode)
	case "corrected":
		return ParsedResult{Raw: wtResp.Corrected}, CheckEmpty(wtResp.Corrected, mode)
	case "shortened":
		return ParsedResult{Raw: wtResp.Shortened}, CheckEmpty(wtResp.Shortened, mode)
	case "expanded":
		return ParsedResult{Raw: wtResp.Expanded}, CheckEmpty(wtResp.Expanded, mode)
	case "bullet":
		return ParsedResult{Raw: wtResp.Bullet}, CheckEmpty(wtResp.Bullet, mode)
	case "table":
		return ParsedResult{Raw: wtResp.Table}, CheckEmpty(wtResp.Table, mode)
	case "casual":
		return ParsedResult{Raw: wtResp.Casual}, CheckEmpty(wtResp.Casual, mode)
	case "funny":
		return ParsedResult{Raw: wtResp.Funny}, CheckEmpty(wtResp.Funny, mode)
	case "confident":
		return ParsedResult{Raw: wtResp.Confident}, CheckEmpty(wtResp.Confident, mode)
	case "straight":
		return ParsedResult{Raw: wtResp.Straight}, CheckEmpty(wtResp.Straight, mode)
	case "professional":
		return ParsedResult{Raw: wtResp.Professional}, CheckEmpty(wtResp.Professional, mode)
	case "translation":
		return ParsedResult{Raw: wtResp.Translation}, CheckEmpty(wtResp.Translation, mode)
	default:
		return ParsedResult{}, fmt.Errorf("field %s is not recognized", fieldName)
	}
}
