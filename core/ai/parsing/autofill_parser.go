package parsing

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pb"
)

// AutofillParser is a ResponseParser for Autofill responses.
type AutofillParser struct {
	// modeToField maps modes to the name of the field in AutofillResponse that should be returned.
	// For example: 1 -> "tag", 2 -> "relation", etc.
	modeToField map[int]string
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
