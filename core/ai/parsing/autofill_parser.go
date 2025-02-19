package parsing

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pb"
)

// AutofillParser is a ResponseParser for Autofill responses.
type AutofillParser struct {
	modeToField  map[int]string
	modeToSchema map[int]func(key string) map[string]interface{}
}

// AutofillResponse represents the structure of the response for different autofill modes.
type AutofillResponse struct {
	Tag         string `json:"tag,omitempty"`
	Relation    string `json:"relation,omitempty"`
	Type        string `json:"type,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// NewAutofillParser returns a new AutofillParser instance.
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

// NewResponseStruct returns a new AutofillResponse instance.
func (p *AutofillParser) NewResponseStruct() interface{} {
	return &AutofillResponse{}
}

// ModeToField returns the modeToField map.
func (p *AutofillParser) ModeToField() map[int]string {
	return p.modeToField
}

// ModeToSchema returns the modeToSchema map.
func (p *AutofillParser) ModeToSchema() map[int]func(key string) map[string]interface{} {
	return p.modeToSchema
}

// ExtractContent extracts the relevant field based on mode.
func (p *AutofillParser) ExtractContent(mode int, response interface{}) (string, error) {
	afResp, ok := response.(*AutofillResponse)
	if !ok {
		return "", fmt.Errorf("invalid response type, expected *AutofillResponse")
	}

	fieldName, exists := p.modeToField[mode]
	if !exists {
		return "", fmt.Errorf("unknown mode: %d", mode)
	}

	// Switch on fieldName to extract
	switch fieldName {
	case "tag":
		return afResp.Tag, CheckEmpty(afResp.Tag, mode)
	case "relation":
		return afResp.Relation, CheckEmpty(afResp.Relation, mode)
	case "type":
		return afResp.Type, CheckEmpty(afResp.Type, mode)
	case "title":
		return afResp.Title, CheckEmpty(afResp.Title, mode)
	case "description":
		return afResp.Description, CheckEmpty(afResp.Description, mode)
	default:
		return "", fmt.Errorf("field %s is not recognized", fieldName)
	}
}
