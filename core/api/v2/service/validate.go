package v2service

// validate.go implements POST /v2/validate: structural + format-semantic
// validation of an AnyBlock document. It exposes
// anyblockjson.Validate directly — the package cannot see the space, so
// referential validation (option names, a type's actual property keys) is
// the space-aware write layer, not this endpoint.

import (
	"encoding/json"
	"errors"
	"fmt"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// ValidateDocument checks an AnyBlock document and returns the issue list
// in the C6 shape. Validation findings are the endpoint's result, not an
// error — the generate → validate → repair loop consumes them as data.
func (s *Service) ValidateDocument(data []byte) v2model.ValidateResponse {
	resp := v2model.ValidateResponse{Issues: []v2model.Issue{}, Warnings: []v2model.Issue{}}
	err := anyblockjson.Validate(data)
	if err == nil {
		return resp
	}

	var validationErr *anyblockjson.ValidationError
	if !errors.As(err, &validationErr) {
		resp.Issues = append(resp.Issues, v2model.Issue{Message: fmt.Sprintf("validate document: %v", err)})
		return resp
	}
	kind := "object"
	if fields, err := parseEnvelope(data); err == nil {
		var docKind string
		if json.Unmarshal(fields["kind"], &docKind) == nil && docKind == "object_type" {
			kind = "type"
		}
	}
	resp.Issues = append(resp.Issues, documentIssues(kind, validationErr.Issues)...)
	if validationErr.NewerFormat {
		// The formatVersion issue already names both
		// versions; the hint steers the repair loop.
		for i := range resp.Issues {
			if resp.Issues[i].Path == "/formatVersion" {
				resp.Issues[i].Hint = "the document was produced by a newer version of the AnyBlock format — upgrade this application or re-export with an older version"
			}
		}
	}
	return resp
}
