package database

import (
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type emptySchema struct{}

func NewEmptySchema() Schema {
	return &emptySchema{}
}

func (sch *emptySchema) RequiredRelations() []*model.RelationLink {
	return []*model.RelationLink{bundle.MustGetRelationLink(bundle.RelationKeyName)}
}

func (sch *emptySchema) ListRelations() []*model.RelationLink {
	return nil
}
