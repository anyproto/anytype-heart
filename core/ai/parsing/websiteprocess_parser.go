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

type WebsiteProcessResponse struct {
	Relations struct{} `json:"relations,omitempty"`
}

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

func (p *WebsiteProcessParser) ModeToField() map[int]string {
	return p.modeToField
}

func (p *WebsiteProcessParser) ModeToSchema() map[int]func(key string) map[string]interface{} {
	return p.modeToSchema
}

func (p *WebsiteProcessParser) ExtractContent(jsonData string, mode int) (ExtractionResult, error) {
	var respMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &respMap); err != nil {
		return ExtractionResult{}, fmt.Errorf("error parsing JSON: %w %s", err, jsonData)
	}

	fieldName, exists := p.modeToField[mode]
	if !exists {
		return ExtractionResult{}, fmt.Errorf("unknown mode: %d", mode)
	}

	fieldValue, exists := respMap[fieldName]
	if !exists {
		return ExtractionResult{}, fmt.Errorf("field %s not found in response", fieldName)
	}

	nestedMap, ok := fieldValue.(map[string]interface{})
	if !ok {
		return ExtractionResult{}, fmt.Errorf("field %s is not an object", fieldName)
	}

	return ExtractionResult{Raw: nestedMap}, nil
}
