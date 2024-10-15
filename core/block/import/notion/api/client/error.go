package client

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/import/common"
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
		return nil
	}
	if notionErr.Status >= 500 {
		return fmt.Errorf("%w: %s", common.ErrNotionServerIsUnavailable, notionErr.Message)
	}
	if notionErr.Status == 429 {
		return fmt.Errorf("%w: %s", common.ErrNotionServerExceedRateLimit, notionErr.Message)
	}
	return fmt.Errorf("status: %d, code: %s, message: %s", notionErr.Status, notionErr.Code, notionErr.Message)
}
