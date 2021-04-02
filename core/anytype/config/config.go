package config

import "github.com/anytypeio/go-anytype-middleware/app"

const CName = "config"

type Config struct {
	NewAccount bool // set to true if a new account is creating. This option controls whether mw should wait for the existing data to arrive before creating the new log
}

func (c Config) Init(a *app.App) (err error) {
	return
}

func (c Config) Name() (name string) {
	return CName
}
