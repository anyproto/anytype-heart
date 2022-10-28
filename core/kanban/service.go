package kanban

import (
	"errors"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)
import "github.com/anytypeio/go-anytype-middleware/app"

const (
	CName = "kanban"
)

func New() Service{
	return &service{groupColumns: make(map[model.RelationFormat]Grouper)}
}

type Grouper interface {
	InitGroups(reqFilters []*model.BlockContentDataviewFilter) error
	MakeGroups() ([]Group, error)
	MakeDataViewGroups() ([]*model.BlockContentDataviewGroup, error)
}

type Service interface {
	Grouper(key string) (Grouper, error)

	app.Component
}

type service struct {
	objectStore objectstore.ObjectStore
	groupColumns map[model.RelationFormat]Grouper
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)

	s.groupColumns[model.RelationFormat_status] = &GroupStatus{store: s.objectStore}
	s.groupColumns[model.RelationFormat_tag] = &GroupTag{store: s.objectStore}
	s.groupColumns[model.RelationFormat_checkbox] = &GroupCheckBox{}

	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Grouper(key string) (Grouper, error) {
	rel, err := s.objectStore.GetRelation(key)
	if err != nil {
		return nil, err
	}

	grouper, ok := s.groupColumns[rel.Format]
	if !ok {
		return nil, errors.New("unsupported relation format")
	}

	return grouper, nil
}


func GroupsToStrSlice(groups []*model.BlockContentDataviewGroup) []string {
	res := make([]string, len(groups))

	for i, g := range groups {
		res[i] = g.Id
	}

	return res
}