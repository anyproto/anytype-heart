package change

import "github.com/anytypeio/go-anytype-middleware/merge/change/chmodel"

type Change struct {
	Id          string
	PreviousIds []string
	Next        []*Change
	Active      bool
	Model       chmodel.Change
}
