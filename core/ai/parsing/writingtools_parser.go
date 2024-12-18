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
	Summary                string `json:"summary,omitempty"`
	Corrected              string `json:"corrected,omitempty"`
	Shortened              string `json:"shortened,omitempty"`
	Expanded               string `json:"expanded,omitempty"`
	Bullet                 string `json:"bullet,omitempty"`
	ContentAsTable         string `json:"content_as_table,omitempty"`
	Translation            string `json:"translation,omitempty"`
	CasualContent          string `json:"casual_content,omitempty"`
	FunnyContent           string `json:"funny_content,omitempty"`
	ConfidentContent       string `json:"confident_content,omitempty"`
	StraightforwardContent string `json:"straightforward_content,omitempty"`
	ProfessionalContent    string `json:"professional_content,omitempty"`
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
			6:  "content_as_table",
			7:  "casual_content",
			8:  "funny_content",
			9:  "confident_content",
			10: "straightforward_content",
			11: "professional_content",
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
	case "content_as_table":
		return wtResp.ContentAsTable, CheckEmpty(wtResp.ContentAsTable, mode)
	case "translation":
		return wtResp.Translation, CheckEmpty(wtResp.Translation, mode)
	case "casual_content":
		return wtResp.CasualContent, CheckEmpty(wtResp.CasualContent, mode)
	case "funny_content":
		return wtResp.FunnyContent, CheckEmpty(wtResp.FunnyContent, mode)
	case "confident_content":
		return wtResp.ConfidentContent, CheckEmpty(wtResp.ConfidentContent, mode)
	case "straightforward_content":
		return wtResp.StraightforwardContent, CheckEmpty(wtResp.StraightforwardContent, mode)
	case "professional_content":
		return wtResp.ProfessionalContent, CheckEmpty(wtResp.ProfessionalContent, mode)
	default:
		return "", fmt.Errorf("field %s is not recognized", fieldName)
	}
}
