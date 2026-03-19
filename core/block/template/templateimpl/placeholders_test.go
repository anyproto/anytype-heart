package templateimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestService_GetTemplatePlaceholders(t *testing.T) {
	const templateId = "template1"

	t.Run("returns placeholders from template", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"dueDate":  domain.String(placeholderToday),
				"assignee": domain.String(placeholderCurrentUser),
			})},
		}, false)

		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		placeholders, err := s.GetTemplatePlaceholders(templateId)

		// then
		require.NoError(t, err)
		require.Len(t, placeholders, 2)

		byKey := make(map[string]*model.Placeholder)
		for _, p := range placeholders {
			byKey[p.RelationKey] = p
		}

		require.Contains(t, byKey, "dueDate")
		require.Len(t, byKey["dueDate"].Values, 1)
		assert.Equal(t, model.Placeholder_PlaceholderToday, byKey["dueDate"].Values[0].Type)

		require.Contains(t, byKey, "assignee")
		require.Len(t, byKey["assignee"].Values, 1)
		assert.Equal(t, model.Placeholder_PlaceholderCurrentUser, byKey["assignee"].Values[0].Type)
	})

	t.Run("returns nil when no placeholders", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String())
		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		placeholders, err := s.GetTemplatePlaceholders(templateId)

		// then
		require.NoError(t, err)
		assert.Nil(t, placeholders)
	})

	t.Run("returns error for non-template object", func(t *testing.T) {
		// given
		sb := smarttest.New(templateId)
		sb.Doc.(*state.State).SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTask})
		s := service{
			picker: &testPicker{sb: sb},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		_, err := s.GetTemplatePlaceholders(templateId)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "object is not a template")
	})
}

func TestService_SetTemplatePlaceholders(t *testing.T) {
	const templateId = "template1"

	t.Run("sets placeholders on template", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday}}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		assert.Equal(t, placeholderToday, m.Get("dueDate").String())
	})

	t.Run("sets multiple placeholders", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday}}},
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser}}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		assert.Equal(t, placeholderToday, m.Get("dueDate").String())
		assert.Equal(t, placeholderCurrentUser, m.Get("assignee").String())
	})

	t.Run("removing placeholder with PlaceholderValue type", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"dueDate":  domain.String(placeholderToday),
				"assignee": domain.String(placeholderCurrentUser),
			})},
		}, false)
		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderValue}}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		assert.True(t, m.Get("dueDate").IsEmpty())
		assert.Equal(t, placeholderCurrentUser, m.Get("assignee").String())
	})

	t.Run("removing last placeholder removes the detail entirely", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"dueDate": domain.String(placeholderToday),
			})},
		}, false)
		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderValue}}},
		})

		// then
		require.NoError(t, err)
		assert.False(t, tmpl.NewState().Details().Has(bundle.RelationKeyTemplatePlaceholders))
	})

	t.Run("removing placeholder with empty values", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"dueDate": domain.String(placeholderToday),
			})},
		}, false)
		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate"},
		})

		// then
		require.NoError(t, err)
		assert.False(t, tmpl.NewState().Details().Has(bundle.RelationKeyTemplatePlaceholders))
	})

	t.Run("non-template objects return error", func(t *testing.T) {
		// given
		sb := smarttest.New(templateId)
		sb.Doc.(*state.State).SetObjectTypeKeys([]domain.TypeKey{bundle.TypeKeyTask})
		s := service{
			picker: &testPicker{sb: sb},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday}}},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "object is not a template")
	})

	t.Run("overwriting existing placeholder preserves others", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"dueDate":  domain.String(placeholderToday),
				"assignee": domain.String(placeholderCurrentUser),
			})},
		}, false)
		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser}}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		assert.Equal(t, placeholderCurrentUser, m.Get("dueDate").String())
		assert.Equal(t, placeholderCurrentUser, m.Get("assignee").String())
	})
}
