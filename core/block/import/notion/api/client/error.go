package client

import (
	"encoding/json"
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

type NotionErrorResponse struct {
	Status  int    `json:"status,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func (ne *NotionErrorResponse) Error() error {
	return fmt.Errorf("status: %d, code: %s, message: %s", ne.Status, ne.Code, ne.Message)
}

func TransformHttpCodeToError(response []byte) *NotionErrorResponse {
	var notionErr = NotionErrorResponse{}
	if err := json.Unmarshal(response, &notionErr); err != nil {
		logging.Logger("client").Error("failed to parse error response from notion %s", err)
		return nil
	}
	return &notionErr
}
