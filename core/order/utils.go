package order

func HasItemNotInSet[T comparable](items []T, set map[T]struct{}) bool {
	for _, id := range items {
		if _, ok := set[id]; !ok {
			return true
		}
	}

	return false
}
