package configfetcher

import (
	"context"

	"github.com/anyproto/any-sync/app"
)

const CName = "configfetcher"

// ConfigFetcher is the offline no-op implementation.
type ConfigFetcher interface {
	app.ComponentRunnable
	Refetch()
}

type configFetcher struct{}

func New() ConfigFetcher {
	return &configFetcher{}
}

func (c *configFetcher) Init(a *app.App) error               { return nil }
func (c *configFetcher) Run(ctx context.Context) error      { return nil }
func (c *configFetcher) Close(ctx context.Context) error    { return nil }
func (c *configFetcher) Name() string                       { return CName }
func (c *configFetcher) Refetch()                           {}
