package namegenerator

import (
	"fmt"
	"math/rand"
)

type NameGenerator struct {
	random            *rand.Rand
	nouns, adjectives []string
}

func (ng *NameGenerator) Generate() string {
	randomAdjective := ng.adjectives[ng.random.Intn(len(ng.adjectives))]
	randomNoun := ng.nouns[ng.random.Intn(len(ng.nouns))]
	return fmt.Sprintf("%v %v", randomAdjective, randomNoun)
}

func NewNameGenerator(seed int64) *NameGenerator {
	return &NameGenerator{
		// nolint:gosec
		random:     rand.New(rand.NewSource(seed)),
		nouns:      getNouns(),
		adjectives: getAdjectives(),
	}
}
