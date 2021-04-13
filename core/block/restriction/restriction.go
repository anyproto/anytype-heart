package restriction

import "github.com/anytypeio/go-anytype-middleware/app"

const CName = "restriction"

type Restriction interface {
	app.Component
}

type restriction struct {
}

func (r *restriction) Init(a *app.App) (err error) {
	return
}

func (r *restriction) Name() (name string) {
	return CName
}

