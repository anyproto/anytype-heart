package database

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSchemaByRelations_ListRelations(t *testing.T) {
	// Given
	common := []*model.RelationLink{
		{
			Key:    bundle.RelationKeyAssignee.String(),
			Format: model.RelationFormat_object,
		},
		{
			Key:    bundle.RelationKeyDueDate.String(),
			Format: model.RelationFormat_date,
		},
	}
	sch := NewByRelations(common)

	// When
	got := sch.ListRelations()

	// Then
	want := []*model.RelationLink{
		{
			Key:    bundle.RelationKeyAssignee.String(),
			Format: model.RelationFormat_object,
		},
		{
			Key:    bundle.RelationKeyDueDate.String(),
			Format: model.RelationFormat_date,
		},
	}
	assert.ElementsMatch(t, want, got)
}

func TestSchemaByRelations_RequiredRelations(t *testing.T) {
	t.Run("hardcoded relations are added", func(t *testing.T) {
		// Given
		common := []*model.RelationLink{
			{
				Key:    bundle.RelationKeyAssignee.String(),
				Format: model.RelationFormat_object,
			},
		}
		sch := NewByRelations(common)

		// When
		got := sch.RequiredRelations()

		// Then
		want := []*model.RelationLink{
			{
				Key:    bundle.RelationKeyName.String(),
				Format: model.RelationFormat_shorttext,
			},
			{
				Key:    bundle.RelationKeyType.String(),
				Format: model.RelationFormat_object,
			},
			{
				Key:    bundle.RelationKeyAssignee.String(),
				Format: model.RelationFormat_object,
			},
		}
		assert.ElementsMatch(t, want, got)
	})
}
