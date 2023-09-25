package client

import (
	"encoding/json"
	"fmt"
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
