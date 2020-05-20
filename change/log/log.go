package log

import "github.com/anytypeio/go-anytype-middleware/pb"

type Log struct {
	Id      string
	Changes map[string]*pb.Change
	Head    string
}

func (l *Log) Get(id string) *pb.Change {
	return l.Changes[id]
}
