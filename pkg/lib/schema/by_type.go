package schema

import (
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type schemaByType struct {
	ObjType *model.ObjectType
}

func NewByType(objType *model.ObjectType) Schema {
	return &schemaByType{ObjType: objType}
}

func (sch *schemaByType) ListRelations() []*model.RelationLink {
	return sch.ObjType.RelationLinks
}

func (sch *schemaByType) RequiredRelations() []*model.RelationLink {
	return []*model.RelationLink{bundle.MustGetRelationLink(bundle.RelationKeyName)}
}
