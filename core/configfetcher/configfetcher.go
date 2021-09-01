package configfetcher

import (
	"context"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	cafeClient "github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-configfetcher")

const CName = "configfetcher"

type ConfigFetcher interface {
	app.Component
	FetchCafeConfig(untilSuccess bool) (*pb.GetConfigResponseConfig, error)
}

type configFetcher struct {
	store objectstore.ObjectStore
	cafe  cafeClient.Client
}

func New() ConfigFetcher {
	return &configFetcher{}
}

func (c *configFetcher) FetchCafeConfig(untilSuccess bool) (*pb.GetConfigResponseConfig, error) {
	var attempt int
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		resp, err := c.cafe.GetConfig(ctx, &pb.GetConfigRequest{})
		cancel()
		if err != nil {
			log.Errorf("failed to request cafe config: %s", err.Error())
		}
		if resp != nil {
			err = c.store.SaveCafeConfig(resp.Config)
			if err != nil {
				log.Errorf("failed to save cafe config to objectstore: %s", err.Error())
				return nil, err
			}
			return resp.Config, nil
		}
		if !untilSuccess {
			return nil, err
		}
		attempt++
		time.Sleep(time.Second * 2 * time.Duration(attempt))
	}
}

func (c *configFetcher) Init(a *app.App) (err error) {
	c.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	c.cafe = a.MustComponent(cafeClient.CName).(cafeClient.Client)
	return nil
}

func (c *configFetcher) Name() (name string) {
	return CName
}
