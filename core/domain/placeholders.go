package domain

import "encoding/json"

const (
	PlaceholderToday       = "_today"
	PlaceholderCurrentUser = "_current_user"
)

// GetPlaceholderValue checks if the given Value is a known placeholder string.
// Returns the placeholder type string if matched, or empty string otherwise.
func GetPlaceholderValue(v Value) string {
	s, ok := v.TryString()
	if !ok {
		return ""
	}
	switch s {
	case PlaceholderToday, PlaceholderCurrentUser:
		return s
	}
	return ""
}

// MarshalPlaceholders serializes a placeholder map to a JSON string.
// Returns empty string if the map is empty.
func MarshalPlaceholders(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// UnmarshalPlaceholders deserializes a JSON string into a placeholder map.
// Returns an empty map if the input is empty or invalid.
func UnmarshalPlaceholders(s string) map[string]string {
	if s == "" {
		return make(map[string]string)
	}
	m := make(map[string]string)
	_ = json.Unmarshal([]byte(s), &m)
	return m
}
