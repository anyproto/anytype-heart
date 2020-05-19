package change

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type Change struct {
	Id   string
	Next []*Change
	*pb.Change
}
