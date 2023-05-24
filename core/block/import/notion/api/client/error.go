package client

import (
	"encoding/json"
	"fmt"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

type NotionErrorResponse struct {
	Status  int    `json:"status,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// TransformHTTPCodeToError creates error based on NotionErrorResponse
func TransformHTTPCodeToError(response []byte) error {
	var notionErr NotionErrorResponse
	if err := json.Unmarshal(response, &notionErr); err != nil {
		logging.Logger("client").Error("failed to parse error response from notion %s", err)
		return nil
	}
	return fmt.Errorf("status: %d, code: %s, message: %s", notionErr.Status, notionErr.Code, notionErr.Message)
}
