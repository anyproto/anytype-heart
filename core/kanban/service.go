package kanban

import (
	"crypto/md5" //nolint:all
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	CName = "kanban"
)

type Service interface {
	Grouper(spaceID string, key string) (Grouper, error)

	app.Component
}

type Grouper interface {
	InitGroups(spaceID string, f *database.Filters) error
	MakeGroups() (GroupSlice, error)
	MakeDataViewGroups() ([]*model.BlockContentDataviewGroup, error)
}

type service struct {
	objectStore  objectstore.ObjectStore
	groupColumns map[model.RelationFormat]func(string) Grouper
}

func New() Service {
	return &service{groupColumns: make(map[model.RelationFormat]func(key string) Grouper)}
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)

	s.groupColumns[model.RelationFormat_status] = func(key string) Grouper {
		return &GroupStatus{key: key, store: s.objectStore}
	}
	s.groupColumns[model.RelationFormat_tag] = func(key string) Grouper {
		return &GroupTag{Key: key, store: s.objectStore}
	}
	s.groupColumns[model.RelationFormat_checkbox] = func(key string) Grouper {
		return &GroupCheckBox{}
	}

	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Grouper(spaceID string, key string) (Grouper, error) {
	rel, err := s.objectStore.SpaceId(spaceID).FetchRelationByKey(key)
	if err != nil {
		return nil, fmt.Errorf("can't get relation %s: %w", key, err)
	}

	grouperFn, ok := s.groupColumns[rel.Format]
	if !ok {
		return nil, errors.New("unsupported relation format")
	}

	return grouperFn(key), nil
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
