package parsing

import (
	"fmt"
)

// WebsiteProcessParser is a ResponseParser for WebsiteProcess responses.
type WebsiteProcessParser struct {
	modeToField  map[int]string
	modeToSchema map[int]func(key string) map[string]interface{}
}

// WebsiteProcessResponse represents the structure of the response for different WebsiteProcess modes.
type WebsiteProcessResponse struct {
	Relations string `json:"relations,omitempty"`
}

// NewWebsiteProcessParser returns a new WebsiteProcessParser instance.
func NewWebsiteProcessParser() *WebsiteProcessParser {
	return &WebsiteProcessParser{
		modeToField: map[int]string{
			1: "relations", // recipe
			2: "relations", // company
			3: "relations", // event
		},
		modeToSchema: map[int]func(key string) map[string]interface{}{
			1: func(key string) map[string]interface{} { // recipe
				fields := map[string]FieldDef{
					"servings":    {Type: "string"},
					"cuisine":     {Type: "string"},
					"cookingTime": {Type: "string"},
					"courseType":  {Type: "string"},
					"difficulty":  {Type: "string"},
				}
				return FlexibleSchema(key, fields, nil)
			},
			2: func(key string) map[string]interface{} { // company
				fields := map[string]FieldDef{
					"name":         {Type: "string"},
					"industry":     {Type: "string"},
					"size":         {Type: "string"},
					"location":     {Type: "string"},
					"foundingYear": {Type: "string"},
				}
				return FlexibleSchema(key, fields, nil)
			},
			3: func(key string) map[string]interface{} { // event
				fields := map[string]FieldDef{
					"name":     {Type: "string"},
					"date":     {Type: "string"},
					"location": {Type: "string"},
					"duration": {Type: "string"},
					"type":     {Type: "string"},
				}
				return FlexibleSchema(key, fields, nil)
			},
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

// ModeToSchema returns the modeToSchema map.
func (p *WebsiteProcessParser) ModeToSchema() map[int]func(key string) map[string]interface{} {
	return p.modeToSchema
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
