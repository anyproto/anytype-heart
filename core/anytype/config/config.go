package config

import "github.com/anytypeio/go-anytype-middleware/app"

const CName = "config"

type Config struct {
	AccountSelect bool
}

func (c Config) Init(a *app.App) (err error) {
	return
}

func (c Config) Name() (name string) {
	return CName
}
