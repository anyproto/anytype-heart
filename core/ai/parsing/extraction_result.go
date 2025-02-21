package parsing

import (
	"encoding/json"
	"fmt"
)

// ExtractionResult represents the result of an extraction operation, which can be either a string or a map.
type ExtractionResult struct {
	Raw interface{}
}

// String returns the parsed result as a JSON-encoded string.
func (er ExtractionResult) String() (string, error) {
	switch v := er.Raw.(type) {
	case string:
		return v, nil
	case map[string]interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	default:
		return "", fmt.Errorf("unexpected type %T", v)
	}
}

// Map returns the parsed result as a map[string]string.
func (er ExtractionResult) Map() (map[string]string, error) {
	switch v := er.Raw.(type) {
	case map[string]interface{}:
		result := make(map[string]string)
		for key, value := range v {
			strValue, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("unexpected value type %T for key %s", value, key)
			}
			result[key] = strValue
		}
		return result, nil
	case string:
		var m map[string]string
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			return nil, err
		}
		return m, nil
	default:
		return nil, fmt.Errorf("unexpected type %T", v)
	}
}
