package subscription

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

// The subscription engine has been removed and awaits a from-scratch
// reimplementation. The contract it must fulfill — derived from how the
// desktop client and anytype-heart's own services consume subscriptions —
// is documented in docs/Subscriptions.md. This file keeps the public API
// surface so that consumers compile; every method returns
// ErrNotImplemented (queries) or is a no-op (teardown).

const CName = "subscription"

// ErrNotImplemented is returned by every query method until the subscription
// engine is reimplemented
var ErrNotImplemented = errors.New("subscriptions are not implemented")

func New() Service {
	return &service{}
}

type SubscribeRequest struct {
	SpaceId string
	SubId   string
	Filters []database.FilterRequest
	Sorts   []database.SortRequest
	Limit   int64
	Offset  int64
	// (required) necessary keys in details for return, for object fields mw will return (and subscribe) objects as dependent
	Keys []string
	// (optional) pagination: middleware will return results after given id
	AfterId string
	// (optional) pagination: middleware will return results before given id
	BeforeId string
	Source   []string
	// disable dependent subscription
	NoDepSubscription bool
	CollectionId      string

	// Internal indicates that subscription will send events into message queue instead of global client's event system
	Internal bool
	// InternalQueue is used when Internal flag is set to true. If it's nil, new queue will be created
	InternalQueue *mb.MB[*pb.EventMessage]
	AsyncInit     bool
}

type SubscribeResponse struct {
	SubId        string
	Records      []*domain.Details
	Dependencies []*domain.Details
	Counters     *pb.EventObjectSubscriptionCounters

	// Used when Internal flag is set to true
	Output *mb.MB[*pb.EventMessage]
}

type SubscribeGroupsRequest struct {
	SpaceId      string
	SubId        string
	RelationKey  string
	Filters      []database.FilterRequest
	Source       []string
	CollectionId string
}

type Service interface {
	Search(req SubscribeRequest) (resp *SubscribeResponse, err error)
	SubscribeIdsReq(req pb.RpcObjectSubscribeIdsRequest) (resp *pb.RpcObjectSubscribeIdsResponse, err error)
	SubscribeIds(subId string, ids []string) (records []*domain.Details, err error)
	SubscribeGroups(req SubscribeGroupsRequest) (*pb.RpcObjectGroupsSubscribeResponse, error)
	Unsubscribe(subIds ...string) (err error)
	UnsubscribeAndReturnIds(spaceId string, subId string) ([]string, error)
	UnsubscribeAll() (err error)
	SubscriptionIDs() []string

	app.ComponentRunnable
}

type CollectionService interface {
	SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error)
	UnsubscribeFromCollection(collectionID string, subscriptionID string) error
}

type service struct{}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	return nil
}

func (s *service) Run(ctx context.Context) (err error) {
	return nil
}

func (s *service) Close(ctx context.Context) error {
	return nil
}

func (s *service) Search(req SubscribeRequest) (resp *SubscribeResponse, err error) {
	return nil, ErrNotImplemented
}

func (s *service) SubscribeIdsReq(req pb.RpcObjectSubscribeIdsRequest) (resp *pb.RpcObjectSubscribeIdsResponse, err error) {
	return nil, ErrNotImplemented
}

func (s *service) SubscribeIds(subId string, ids []string) (records []*domain.Details, err error) {
	return nil, ErrNotImplemented
}

func (s *service) SubscribeGroups(req SubscribeGroupsRequest) (*pb.RpcObjectGroupsSubscribeResponse, error) {
	return nil, ErrNotImplemented
}

func (s *service) Unsubscribe(subIds ...string) (err error) {
	return nil
}

func (s *service) UnsubscribeAndReturnIds(spaceId string, subId string) ([]string, error) {
	return nil, ErrNotImplemented
}

func (s *service) UnsubscribeAll() (err error) {
	return nil
}

func (s *service) SubscriptionIDs() []string {
	return nil
}
