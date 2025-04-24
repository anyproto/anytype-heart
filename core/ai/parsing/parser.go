package parsing

import (
	"fmt"
)

// ResponseParser defines how to parse and extract content from the model's JSON response.
// It abstracts away the differences between different types of tasks (e.g., WritingTools vs. Autofill).
type ResponseParser interface {
	// ModeToField maps modes to the name of the field in the response struct that should be returned.
	ModeToField() map[int]string

	// ModeToSchema maps modes to the structure of the response schema.
	ModeToSchema() map[int]func(key string) map[string]interface{}

	// ExtractContent uses the mode and the already-unmarshalled response struct to return the final answer string.
	ExtractContent(jsonData string, mode int) (ExtractionResult, error)
}

// checkEmpty checks if the content is empty and returns an error if it is.
func checkEmpty(content string, mode int) error {
	if content == "" {
		return fmt.Errorf("content is empty for mode %d", mode)
	}
	return nil
}
