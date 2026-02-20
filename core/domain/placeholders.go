package domain

const (
	PlaceholderToday       = "_today"
	PlaceholderCurrentUser = "_current_user"
)

func IsPlaceholder(v Value) bool {
	s, ok := v.TryString()
	if !ok {
		return false
	}
	switch s {
	case PlaceholderToday, PlaceholderCurrentUser:
		return true
	}
	return false
}
