package parsing

import "fmt"

// ResponseParser defines how to parse and extract content from the model's JSON response.
// It abstracts away the differences between different types of tasks (e.g., WritingTools vs. Autofill).
type ResponseParser interface {
	// NewResponseStruct returns a new instance of the response structure into which JSON can be unmarshalled.
	NewResponseStruct() interface{}

	// ModeToField maps modes to the name of the field in the response struct that should be returned.
	ModeToField() map[int]string

	// ExtractContent uses the mode and the already-unmarshalled response struct to
	// return the final answer string. Returns an error if extraction fails.
	ExtractContent(mode int, response interface{}) (string, error)
}

// CheckEmpty checks if the content is empty and returns an error if it is.
func CheckEmpty(content string, mode int) error {
	if content == "" {
		return fmt.Errorf("content is empty for mode %d", mode)
	}
	return nil
}
