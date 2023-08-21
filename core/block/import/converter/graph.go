package converter

type Neighbors map[string]struct{}

type Graph map[string]Neighbors
