package typesuggest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSuggestByContainerName(t *testing.T) {
	cases := []struct {
		name string
		want domain.TypeKey
	}{
		{"Tasks", bundle.TypeKeyTask},
		{"✅ To-Do", bundle.TypeKeyTask},
		{"to-dos", bundle.TypeKeyTask},
		{"Backlog", bundle.TypeKeyTask},
		{"  Projects ", bundle.TypeKeyProject},
		{"People", bundle.TypeKeyContact},
		{"Team Members", bundle.TypeKeyContact},
		{"Reading List", bundle.TypeKeyBook},
		{"📔 Journal", bundle.TypeKeyDiaryEntry},
		{"OKRs", bundle.TypeKeyGoal},
		{"Recipes", bundle.TypeKeyRecipe},
		{"Movies", bundle.TypeKeyMovie},
		{"Meeting Notes", ""}, // compound names don't match — conservative
		{"Stuff", ""},
		{"Задачи", ""}, // non-english: no naive match, ML iteration territory
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// given / when
			suggestion, ok := NewNaive().Suggest(Evidence{ContainerName: tc.name})

			// then
			if tc.want == "" {
				assert.False(t, ok)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tc.want, suggestion.TypeKey)
			assert.Equal(t, "container name", suggestion.Reason)
			assert.GreaterOrEqual(t, suggestion.Confidence, 0.7)
		})
	}
}

func TestSuggestByPropertyShape(t *testing.T) {
	t.Run("email and phone mean contact", func(t *testing.T) {
		// given / when
		suggestion, ok := NewNaive().Suggest(Evidence{
			ContainerName: "CRM",
			Properties: []Property{
				{Name: "Email", Format: model.RelationFormat_email},
				{Name: "Phone", Format: model.RelationFormat_phone},
				{Name: "Company", Format: model.RelationFormat_shorttext},
			},
		})

		// then
		require.True(t, ok)
		assert.Equal(t, bundle.TypeKeyContact, suggestion.TypeKey)
	})

	t.Run("completion checkbox means task", func(t *testing.T) {
		// given / when
		suggestion, ok := NewNaive().Suggest(Evidence{
			ContainerName: "Q3 Sprint",
			Properties: []Property{
				{Name: "Done", Format: model.RelationFormat_checkbox},
			},
		})

		// then
		require.True(t, ok)
		assert.Equal(t, bundle.TypeKeyTask, suggestion.TypeKey)
	})

	t.Run("due date plus status means task", func(t *testing.T) {
		// given / when
		suggestion, ok := NewNaive().Suggest(Evidence{
			ContainerName: "Q3 Sprint",
			Properties: []Property{
				{Name: "Due Date", Format: model.RelationFormat_date},
				{Name: "Stage", Format: model.RelationFormat_status},
			},
		})

		// then
		require.True(t, ok)
		assert.Equal(t, bundle.TypeKeyTask, suggestion.TypeKey)
	})

	t.Run("weak shapes stay unsuggested", func(t *testing.T) {
		// given — a checkbox that is not completion-like, a date that is
		// not a due date
		_, ok := NewNaive().Suggest(Evidence{
			ContainerName: "Inventory",
			Properties: []Property{
				{Name: "In Stock", Format: model.RelationFormat_checkbox},
				{Name: "Added", Format: model.RelationFormat_date},
			},
		})

		// then
		assert.False(t, ok)
	})

	t.Run("container name outranks shape", func(t *testing.T) {
		// given — a contacts database that also has a Done checkbox
		suggestion, ok := NewNaive().Suggest(Evidence{
			ContainerName: "Contacts",
			Properties: []Property{
				{Name: "Done", Format: model.RelationFormat_checkbox},
			},
		})

		// then
		require.True(t, ok)
		assert.Equal(t, bundle.TypeKeyContact, suggestion.TypeKey)
	})
}
