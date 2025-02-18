package parsing

import (
	"fmt"
)

// WebsiteProcessParser is a ResponseParser for WebsiteProcess responses.
type WebsiteProcessParser struct {
	// modeToField maps modes to the name of the field in WebsiteProcessResponse that should be returned.
	// For example: 1 -> "relation", etc.
	modeToField map[int]string
}

// WebsiteProcessResponse represents the structure of the response for different WebsiteProcess modes.
type WebsiteProcessResponse struct {
	Relations string `json:"relations,omitempty"`
}

// NewWebsiteProcessParser returns a new WebsiteProcessParser instance.
func NewWebsiteProcessParser() *WebsiteProcessParser {
	return &WebsiteProcessParser{
		modeToField: map[int]string{
			1: "relations",
		},
	}
}

// NewResponseStruct returns a new WebsiteProcessResponse instance.
func (p *WebsiteProcessParser) NewResponseStruct() interface{} {
	return &WebsiteProcessResponse{}
}

// ModeToField returns the modeToField map.
func (p *WebsiteProcessParser) ModeToField() map[int]string {
	return p.modeToField
}

// ExtractContent extracts the relevant field based on mode.
func (p *WebsiteProcessParser) ExtractContent(mode int, response interface{}) (string, error) {
	afResp, ok := response.(*WebsiteProcessResponse)
	if !ok {
		return "", fmt.Errorf("invalid response type, expected *WebsiteProcessResponse")
	}

	fieldName, exists := p.modeToField[mode]
	if !exists {
		return "", fmt.Errorf("unknown mode: %d", mode)
	}

	// Switch on fieldName to extract
	switch fieldName {
	case "relations":
		return afResp.Relations, CheckEmpty(afResp.Relations, mode)
	default:
		return "", fmt.Errorf("field %s is not recognized", fieldName)
	}
}
