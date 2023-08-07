package schema

import (
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"testing"
)

func givenSchemaByType() Schema {
	relations := []*model.RelationLink{
		bundle.MustGetRelationLink(bundle.RelationKeyPriority),
		bundle.MustGetRelationLink(bundle.RelationKeyBudget),
	}
	objType := bundle.MustGetType(bundle.TypeKeyProject)
	return NewByType(objType, relations)
}

func TestSchemaByType_RequiredRelations(t *testing.T) {
	// Given
	sch := givenSchemaByType()

	// When
	got := sch.RequiredRelations()

	// Then
	want := []*model.RelationLink{
		// Relations from schema are ignored
		// Relation links from type are also ignored
		bundle.MustGetRelationLink(bundle.RelationKeyName),
	}
	assert.ElementsMatch(t, want, got)
}

func TestSchemaByType_ListRelations(t *testing.T) {
	// Given
	sch := givenSchemaByType()

	// When
	got := sch.ListRelations()

	// Then
	want := append(
		// Relations from schema
		[]*model.RelationLink{
			bundle.MustGetRelationLink(bundle.RelationKeyPriority),
			// Budget relation is also included in RelationLinks for object type,
			// so we have to remove duplicates
			bundle.MustGetRelationLink(bundle.RelationKeyBudget),
		},
		// Relations from type
		bundle.MustGetType(bundle.TypeKeyProject).RelationLinks...,
	)
	want = lo.UniqBy(want, func(item *model.RelationLink) string {
		return item.Key
	})

	assert.ElementsMatch(t, want, got)
}

func TestSchemaByType_Filters(t *testing.T) {
	// Given
	sch := givenSchemaByType()

	// When
	got := sch.Filters()

	// Then
	want := filter.OrFilters{
		filter.Eq{
			Key:   bundle.RelationKeyType.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.String(bundle.TypeKeyProject.BundledURL()),
		},
	}
	assert.Equal(t, want, got)
}
