package dependencies

import "github.com/anyproto/anytype-heart/pkg/lib/database"

type QueryableStore interface {
	Query(q database.Query) (records []database.Record, err error)
}
