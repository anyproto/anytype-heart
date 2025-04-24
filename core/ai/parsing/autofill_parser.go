package parsing

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/pb"
)

// AutofillParser is a ResponseParser for Autofill responses.
type AutofillParser struct {
	modeToField  map[int]string
	modeToSchema map[int]func(key string) map[string]interface{}
}

type AutofillResponse struct {
	Tag         string `json:"tag,omitempty"`
	Relation    string `json:"relation,omitempty"`
	Type        string `json:"type,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

func NewAutofillParser() *AutofillParser {
	return &AutofillParser{
		modeToField: map[int]string{
			int(pb.RpcAIAutofillRequest_TAG):         "tag",
			int(pb.RpcAIAutofillRequest_RELATION):    "relation",
			int(pb.RpcAIAutofillRequest_TYPE):        "type",
			int(pb.RpcAIAutofillRequest_TITLE):       "title",
			int(pb.RpcAIAutofillRequest_DESCRIPTION): "description",
		},
		modeToSchema: map[int]func(key string) map[string]interface{}{
			int(pb.RpcAIAutofillRequest_TAG):         SingleStringSchema,
			int(pb.RpcAIAutofillRequest_RELATION):    SingleStringSchema,
			int(pb.RpcAIAutofillRequest_TYPE):        SingleStringSchema,
			int(pb.RpcAIAutofillRequest_TITLE):       SingleStringSchema,
			int(pb.RpcAIAutofillRequest_DESCRIPTION): SingleStringSchema,
		},
	}
}

func (p *AutofillParser) ModeToField() map[int]string {
	return p.modeToField
}

func (p *AutofillParser) ModeToSchema() map[int]func(key string) map[string]interface{} {
	return p.modeToSchema
}

func (p *AutofillParser) ExtractContent(jsonData string, mode int) (ExtractionResult, error) {
	var afResp AutofillResponse
	if err := json.Unmarshal([]byte(jsonData), &afResp); err != nil {
		return ExtractionResult{}, fmt.Errorf("error parsing JSON: %w %s", err, jsonData)
	}

	fieldName, exists := p.modeToField[mode]
	if !exists {
		return ExtractionResult{}, fmt.Errorf("unknown mode: %d", mode)
	}

	switch fieldName {
	case "tag":
		return ExtractionResult{Raw: afResp.Tag}, checkEmpty(afResp.Tag, mode)
	case "relation":
		return ExtractionResult{Raw: afResp.Relation}, checkEmpty(afResp.Relation, mode)
	case "type":
		return ExtractionResult{Raw: afResp.Type}, checkEmpty(afResp.Type, mode)
	case "title":
		return ExtractionResult{Raw: afResp.Title}, checkEmpty(afResp.Title, mode)
	case "description":
		return ExtractionResult{Raw: afResp.Description}, checkEmpty(afResp.Description, mode)
	default:
		return ExtractionResult{}, fmt.Errorf("field %s is not recognized", fieldName)
	}
}
