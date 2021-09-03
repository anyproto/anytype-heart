package configfetcher

import (
	"context"
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	cafeClient "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-configfetcher")

const CName = "configfetcher"

var defaultConfigResponse = &pb.GetConfigResponseConfig{
	EnableDataview:             false,
	EnableDebug:                false,
	EnableReleaseChannelSwitch: false,
	SimultaneousRequests:       20,
	Extra:                      nil,
}

type ConfigFetcher interface {
	app.ComponentRunnable
	// GetConfig fetches the config or returns default after context is done
	GetConfig(ctx context.Context) *pb.GetConfigResponseConfig
}

type configFetcher struct {
	store objectstore.ObjectStore
	cafe  cafeClient.Client

	fetched chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
}

func New() ConfigFetcher {
	return &configFetcher{}
}

func (c *configFetcher) Run() error {
	c.ctx, c.cancel = context.WithCancel(context.Background())
	go func() {
		var attempt int
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(time.Second * 2 * time.Duration(attempt)):
			}
			err := c.fetchConfig()
			if err == nil {
				close(c.fetched)
				return
			}

			attempt++
			log.Errorf("failed to fetch cafe config after %d attempts with error: %s", attempt, err.Error())
		}
	}()
	return nil
}

func (c *configFetcher) GetConfig(ctx context.Context) *pb.GetConfigResponseConfig {
	select {
	case <-c.fetched:
	case <-ctx.Done():
	}

	cfg, err := c.store.GetCafeConfig()
	if err != nil {
		log.Errorf("failed to get cafe config from the store: %s", err.Error())
		cfg = defaultConfigResponse
	}
	return cfg
}

func (c *configFetcher) Init(a *app.App) (err error) {
	c.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	c.cafe = a.MustComponent(cafeClient.CName).(cafeClient.Client)
	c.fetched = make(chan struct{})
	return nil
}

func (c *configFetcher) Name() (name string) {
	return CName
}

func (c *configFetcher) fetchConfig() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	resp, err := c.cafe.GetConfig(ctx, &pb.GetConfigRequest{})
	cancel()
	if err != nil {
		return fmt.Errorf("failed to request cafe config: %w", err)
	}
	if resp != nil {
		err = c.store.SaveCafeConfig(resp.Config)
		if err != nil {
			return fmt.Errorf("failed to save cafe config to objectstore: %w", err)
		}
	}
	return err
}

func (c *configFetcher) Close() (err error) {
	c.cancel()
	return nil
}
