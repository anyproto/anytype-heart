package subscription

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
)

const CName = "subscription"

type Service interface {
	Search(req pb.RpcObjectSearchSubscribeRequest) (resp *pb.RpcObjectSearchResponse, err error)
	SubscribeIds(subId string, ids []string) (records []*types.Struct, err error)
	Unsubscribe(subId string) (err error)
	UnsubscribeAll() (err error)

	app.ComponentRunnable
}

type service struct {
	cache *cache
}

func (s *service) Search(req pb.RpcObjectSearchSubscribeRequest) (resp *pb.RpcObjectSearchResponse, err error) {
	return
}

func (s *service) SubscribeIds(subId string, ids []string) (records []*types.Struct, err error) {
	return
}

func (s *service) Unsubscribe(subId string) (err error) {
	return
}

func (s *service) UnsubscribeAll() (err error) {
	return
}

func (s *service) Init(a *app.App) (err error) {
	s.cache = newCache()
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run() (err error) {
	return
}

func (s *service) Close() (err error) {
	return
}
