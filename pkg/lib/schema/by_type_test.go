package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func givenSchemaByType() Schema {
	objType := bundle.MustGetType(bundle.TypeKeyProject)
	return NewByType(objType)
}

func TestSchemaByType_RequiredRelations(t *testing.T) {
	// Given
	sch := givenSchemaByType()

	// When
	got := sch.RequiredRelations()

	// Then
	want := []*model.RelationLink{
		// Relation links from type are ignored
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
	want := bundle.MustGetType(bundle.TypeKeyProject).RelationLinks

	assert.ElementsMatch(t, want, got)
}
