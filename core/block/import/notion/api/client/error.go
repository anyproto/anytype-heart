package client

import (
	"encoding/json"
	"fmt"
	"strconv"

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
		return nil
	}
	return fmt.Errorf("status: %d, code: %s, message: %s", notionErr.Status, notionErr.Code, notionErr.Message)
}

func GetErrorCode(response []byte) int64 {
	var notionErr NotionErrorResponse
	if err := json.Unmarshal(response, &notionErr); err != nil {
		logging.Logger("client").Error("failed to parse error response from notion %s", err)
		return 0
	}
	code, err := strconv.ParseInt(notionErr.Code, 10, 64)
	if err != nil {
		return 0
	}
	return code
}
