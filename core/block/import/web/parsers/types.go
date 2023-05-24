package parsers

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

type RegisterParser func() Parser

var Parsers []RegisterParser

func RegisterFunc(p RegisterParser) {
	Parsers = append(Parsers, p)
}

type Parser interface {
	ParseUrl(url string) (*model.SmartBlockSnapshotBase, error)
	MatchUrl(url string) bool
}
