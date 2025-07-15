package namegenerator

import (
	"fmt"
	"math/rand"
)

type Generator interface {
	Generate() string
}

type NameGenerator struct {
	random            *rand.Rand
	nouns, adjectives []string
}

func (ng *NameGenerator) Generate() string {
	randomAdjective := ng.adjectives[ng.random.Intn(len(ng.adjectives))]
	randomNoun := ng.nouns[ng.random.Intn(len(ng.nouns))]

	randomName := fmt.Sprintf("%v %v", randomAdjective, randomNoun)

	return randomName
}

func NewNameGenerator(seed int64) Generator {
	nameGenerator := &NameGenerator{
		random:     rand.New(rand.New(rand.NewSource(99))),
		nouns:      getNouns(),
		adjectives: getAdjectives(),
	}
	nameGenerator.random.Seed(seed)

	return nameGenerator
}
