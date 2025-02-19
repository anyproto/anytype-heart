package parsing

import (
	"encoding/json"
	"fmt"
)

// WebsiteProcessParser is a ResponseParser for WebsiteProcess responses.
type WebsiteProcessParser struct {
	modeToField  map[int]string
	modeToSchema map[int]func(key string) map[string]interface{}
}

// WebsiteProcessResponse represents the structure of the response for different WebsiteProcess modes.
type WebsiteProcessResponse struct {
	Relations struct{} `json:"relations,omitempty"`
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
				return BuildJSONSchema(key, map[string]interface{}{
					"servings":    "string",
					"cuisine":     "string",
					"cookingTime": "string",
					"courseType":  "string",
					"difficulty":  "string",
				})
			},
			2: func(key string) map[string]interface{} { // company
				return BuildJSONSchema(key, map[string]interface{}{
					"name":         "string",
					"industry":     "string",
					"size":         "string",
					"location":     "string",
					"foundingYear": "string",
				})
			},
			3: func(key string) map[string]interface{} { // event
				return BuildJSONSchema(key, map[string]interface{}{
					"name":     "string",
					"date":     "string",
					"location": "string",
					"duration": "string",
					"type":     "string",
				})
			},
		},
	}
}

// NewResponseStruct returns a new WebsiteProcessResponse instance.
func (p *WebsiteProcessParser) NewResponseStruct() interface{} {
	var genericResponse map[string]interface{}
	return &genericResponse
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
func (p *WebsiteProcessParser) ExtractContent(jsonData string, mode int) (ParsedResult, error) {
	respStruct := p.NewResponseStruct()

	err := json.Unmarshal([]byte(jsonData), &respStruct)
	if err != nil {
		return ParsedResult{}, fmt.Errorf("error parsing JSON: %w %s", err, jsonData)
	}

	respMap, ok := respStruct.(*map[string]interface{})
	if !ok {
		return ParsedResult{}, fmt.Errorf("invalid response type, expected *map[string]interface{}")
	}

	fieldName, exists := p.modeToField[mode]
	if !exists {
		return ParsedResult{}, fmt.Errorf("unknown mode: %d", mode)
	}

	fieldValue, exists := (*respMap)[fieldName]
	if !exists {
		return ParsedResult{}, fmt.Errorf("field %s not found in response", fieldName)
	}

	nestedMap, ok := fieldValue.(map[string]interface{})
	if !ok {
		return ParsedResult{}, fmt.Errorf("field %s is not an object", fieldName)
	}

	return ParsedResult{Raw: nestedMap}, nil
}
