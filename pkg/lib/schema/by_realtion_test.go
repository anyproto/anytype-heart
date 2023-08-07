package schema

import (
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSchemaByRelations_ListRelations(t *testing.T) {
	t.Run("intersecting common and optional relations", func(t *testing.T) {
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
		optional := []*model.RelationLink{
			{
				Key:    bundle.RelationKeyDueDate.String(),
				Format: model.RelationFormat_date,
			},
			{
				Key:    bundle.RelationKeyPriority.String(),
				Format: model.RelationFormat_number,
			},
		}
		sch := NewByRelations(nil, common, optional)

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
			{
				Key:    bundle.RelationKeyPriority.String(),
				Format: model.RelationFormat_number,
			},
		}
		assert.ElementsMatch(t, want, got)
	})

	t.Run("provided object types are ignored", func(t *testing.T) {
		// Given
		sch := NewByRelations([]string{"derivedFrom(ot-note)", "derivedFrom(ot-page)"}, nil, nil)
		// When
		got := sch.ListRelations()
		// Then
		assert.Empty(t, got)
	})
}

func TestSchemaByRelations_RequiredRelations(t *testing.T) {
	t.Run("hardcoded relation added and optional relations are ignored", func(t *testing.T) {
		// Given
		common := []*model.RelationLink{
			{
				Key:    bundle.RelationKeyAssignee.String(),
				Format: model.RelationFormat_object,
			},
		}
		optional := []*model.RelationLink{
			{
				Key:    bundle.RelationKeyDueDate.String(),
				Format: model.RelationFormat_date,
			},
			{
				Key:    bundle.RelationKeyPriority.String(),
				Format: model.RelationFormat_number,
			},
		}
		sch := NewByRelations(nil, common, optional)

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

	t.Run("provided object types are ignored, returns only hardcoded relations", func(t *testing.T) {
		// Given
		sch := NewByRelations([]string{"derivedFrom(ot-note)", "derivedFrom(ot-page)"}, nil, nil)

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
		}
		assert.ElementsMatch(t, want, got)
	})
}

func TestSchemaByRelations_Filters(t *testing.T) {
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
	// Optional relations are ignored
	optional := []*model.RelationLink{
		{
			Key:    bundle.RelationKeyPriority.String(),
			Format: model.RelationFormat_number,
		},
	}
	objectTypes := []string{"derivedFrom(ot-note)", "derivedFrom(ot-page)"}
	sch := NewByRelations(objectTypes, common, optional)

	// When
	got := sch.Filters()

	// Then
	want := filter.OrFilters{
		filter.In{
			Key:   bundle.RelationKeyType.String(),
			Value: pbtypes.StringList(objectTypes).GetListValue(),
		},
		filter.Exists{
			Key: bundle.RelationKeyAssignee.String(),
		},
		filter.Exists{
			Key: bundle.RelationKeyDueDate.String(),
		},
	}
	assert.Equal(t, want, got)
}
