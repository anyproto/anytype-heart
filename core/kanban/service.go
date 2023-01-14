package kanban

import (
	"crypto/md5" //nolint:all
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)
import "github.com/anytypeio/go-anytype-middleware/app"

const (
	CName = "kanban"
)

func New() Service {
	return &service{groupColumns: make(map[model.RelationFormat]Grouper)}
}

type Grouper interface {
	InitGroups(f *database.Filters) error
	MakeGroups() (GroupSlice, error)
	MakeDataViewGroups() ([]*model.BlockContentDataviewGroup, error)
}

type Service interface {
	Grouper(key string) (Grouper, error)

	app.Component
}

type service struct {
	objectStore  objectstore.ObjectStore
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
	rel, err := s.objectStore.GetRelationByKey(key)
	if err != nil {

		return nil, fmt.Errorf("can't get relation %s: %v", key, err)
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

func Hash(id string) string {
	hash := md5.Sum([]byte(id)) //nolint:gosec
	idHash := hex.EncodeToString(hash[:])
	return idHash
}
