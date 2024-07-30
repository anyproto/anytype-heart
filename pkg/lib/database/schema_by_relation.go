package database

import (
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type schemaByRelations struct {
	CommonRelations []*model.RelationLink
}

func NewByRelations(commonRelations []*model.RelationLink) Schema {
	return &schemaByRelations{CommonRelations: commonRelations}
}

func (sch *schemaByRelations) RequiredRelations() []*model.RelationLink {
	required := []*model.RelationLink{
		bundle.MustGetRelationLink(bundle.RelationKeyName),
		bundle.MustGetRelationLink(bundle.RelationKeyType),
	}
	required = append(required, sch.CommonRelations...)
	return lo.UniqBy(required, func(rel *model.RelationLink) string {
		return rel.Key
	})
}

func (sch *schemaByRelations) ListRelations() []*model.RelationLink {
	return sch.CommonRelations
}
