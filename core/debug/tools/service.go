package tools

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/anyproto/any-sync/app"
	"github.com/go-chi/chi/v5"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/debug"
)

const CName = "core.debug.tools"

type Service interface {
	app.Component
}

func New() Service {
	return &service{}
}

type service struct {
	objectStore  objectstore.ObjectStore
	spaceService space.Service
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.spaceService = app.MustComponent[space.Service](a)
	return nil
}

func (s *service) DebugRouter(r chi.Router) {
	r.Get("/spaceViews", debug.JSONHandler(s.spaceViewsHandler))
}

func parseValue(raw string, format model.RelationFormat) (domain.Value, error) {
	switch format {
	case model.RelationFormat_number:
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return domain.Value{}, err
		}
		return domain.Int64(v), nil
	case model.RelationFormat_shorttext, model.RelationFormat_longtext:
		return domain.String(raw), nil
	case model.RelationFormat_checkbox:
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return domain.Value{}, err
		}
		return domain.Bool(v), nil
	default:
		return domain.Value{}, fmt.Errorf("invalid format")
	}
}

func (s *service) spaceViewsHandler(req *http.Request) (interface{}, error) {
	var filters []database.FilterRequest
	for key, vals := range req.URL.Query() {
		if len(vals) == 0 {
			continue
		}

		rkey := domain.RelationKey(key)
		rel, err := bundle.GetRelation(rkey)
		if err != nil {
			return domain.Invalid(), err
		}
		val, err := parseValue(vals[0], rel.Format)
		if err != nil {
			continue
		}
		filters = append(filters, database.FilterRequest{
			RelationKey: rkey,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       val,
		})
	}

	filters = append(filters, database.FilterRequest{
		RelationKey: bundle.RelationKeyResolvedLayout,
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       domain.Int64(model.ObjectType_spaceView),
	})

	techSpaceId := s.spaceService.TechSpaceId()
	views, err := s.objectStore.SpaceIndex(techSpaceId).Query(database.Query{
		Filters: filters,
	})
	if err != nil {
		return nil, err
	}

	viewsList := make([]*domain.Details, len(views))
	for i, view := range views {
		viewsList[i] = view.Details
	}

	return struct {
		Views []*domain.Details
	}{
		Views: viewsList,
	}, nil
}
