package maputils

func CopyMap[K comparable, V any](original map[K]V) map[K]V {
	copiedMap := make(map[K]V, len(original))
	for key, value := range original {
		copiedMap[key] = value
	}
	return copiedMap
}
