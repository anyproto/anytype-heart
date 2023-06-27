package testapp

import (
	"github.com/anyproto/any-sync/app"
)

func New() *TestApp {
	return &TestApp{&app.App{}}
}

type TestApp struct {
	*app.App
}

func (ta *TestApp) With(cmp app.Component) *TestApp {
	ta.Register(cmp)
	return ta
}
