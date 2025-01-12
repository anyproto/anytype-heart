package parsing

import (
	"fmt"
)

// WritingToolsParser is a ResponseParser for WritingTools responses.
// It knows how to extract the correct field from the WritingToolsResponse based on the mode.
type WritingToolsParser struct {
	// modeToField maps modes to the name of the field in WritingToolsResponse that should be returned.
	// For example: 1 -> "Summary", 2 -> "Corrected", etc.
	modeToField map[int]string
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
			1:  "summary",
			2:  "corrected",
			3:  "shortened",
			4:  "expanded",
			5:  "bullet",
			6:  "table",
			7:  "casual",
			8:  "funny",
			9:  "confident",
			10: "straight",
			11: "professional",
			12: "translation",
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
