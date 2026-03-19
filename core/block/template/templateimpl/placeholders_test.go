package templateimpl

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestService_GetTemplatePlaceholders(t *testing.T) {
	const templateId = "template1"

	t.Run("returns shortcut placeholders from template", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"dueDate":  storageShortcut(model.Placeholder_PlaceholderToday),
				"assignee": storageShortcut(model.Placeholder_PlaceholderCurrentUser),
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

	t.Run("returns concrete value placeholder", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"priority": domain.NewValueMap(map[string]domain.Value{
					"0": domain.Float64(42),
				}),
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
		require.Len(t, placeholders, 1)
		assert.Equal(t, "priority", placeholders[0].RelationKey)
		require.Len(t, placeholders[0].Values, 1)
		assert.Equal(t, model.Placeholder_PlaceholderValue, placeholders[0].Values[0].Type)
		assert.Equal(t, float64(42), placeholders[0].Values[0].Value.GetNumberValue())
	})

	t.Run("returns combined concrete and shortcut placeholder", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"assignee": domain.NewValueMap(map[string]domain.Value{
					"0": domain.StringList([]string{"obj1"}),
					"2": domain.Bool(true),
				}),
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
		require.Len(t, placeholders, 1)
		assert.Equal(t, "assignee", placeholders[0].RelationKey)
		require.Len(t, placeholders[0].Values, 2)

		byType := make(map[model.PlaceholderType]*model.PlaceholderValue)
		for _, v := range placeholders[0].Values {
			byType[v.Type] = v
		}

		require.Contains(t, byType, model.Placeholder_PlaceholderValue)
		assert.Equal(t, pbtypes.StringList([]string{"obj1"}), byType[model.Placeholder_PlaceholderValue].Value)

		require.Contains(t, byType, model.Placeholder_PlaceholderCurrentUser)
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

	t.Run("sets shortcut placeholder on template", func(t *testing.T) {
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
		inner, ok := m.TryMapValue("dueDate")
		require.True(t, ok)
		assert.True(t, inner.Has("1"))
	})

	t.Run("sets concrete value placeholder", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.StringList([]string{"user1", "user2"})},
			}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		inner, ok := m.TryMapValue("assignee")
		require.True(t, ok)
		assert.Equal(t, []string{"user1", "user2"}, inner.GetStringList("0"))
	})

	t.Run("sets combined concrete and shortcut placeholders", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.StringList([]string{"obj1"})},
				{Type: model.Placeholder_PlaceholderCurrentUser},
			}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		inner, ok := m.TryMapValue("assignee")
		require.True(t, ok)
		assert.Equal(t, []string{"obj1"}, inner.GetStringList("0"))
		assert.True(t, inner.Has("2"))
	})

	t.Run("sets multiple relation placeholders", func(t *testing.T) {
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
		dueDateInner, ok := m.TryMapValue("dueDate")
		require.True(t, ok)
		assert.True(t, dueDateInner.Has("1"))

		assigneeInner, ok := m.TryMapValue("assignee")
		require.True(t, ok)
		assert.True(t, assigneeInner.Has("2"))
	})

	t.Run("removing placeholder with PlaceholderValue type", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"dueDate":  storageShortcut(model.Placeholder_PlaceholderToday),
				"assignee": storageShortcut(model.Placeholder_PlaceholderCurrentUser),
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

		assigneeInner, ok := m.TryMapValue("assignee")
		require.True(t, ok)
		assert.True(t, assigneeInner.Has("2"))
	})

	t.Run("removing last placeholder removes the detail entirely", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"dueDate": storageShortcut(model.Placeholder_PlaceholderToday),
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
				"dueDate": storageShortcut(model.Placeholder_PlaceholderToday),
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
				"dueDate":  storageShortcut(model.Placeholder_PlaceholderToday),
				"assignee": storageShortcut(model.Placeholder_PlaceholderCurrentUser),
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

		dueDateInner, ok := m.TryMapValue("dueDate")
		require.True(t, ok)
		assert.True(t, dueDateInner.Has("2"))

		assigneeInner, ok := m.TryMapValue("assignee")
		require.True(t, ok)
		assert.True(t, assigneeInner.Has("2"))
	})
}

// storageShortcut helper is defined in impl_test.go

func TestShouldRemovePlaceholder(t *testing.T) {
	t.Run("empty values means remove", func(t *testing.T) {
		assert.True(t, shouldRemovePlaceholder(nil))
		assert.True(t, shouldRemovePlaceholder([]*model.PlaceholderValue{}))
	})

	t.Run("PlaceholderValue with nil value means remove", func(t *testing.T) {
		assert.True(t, shouldRemovePlaceholder([]*model.PlaceholderValue{
			{Type: model.Placeholder_PlaceholderValue},
		}))
	})

	t.Run("PlaceholderValue with actual value means keep", func(t *testing.T) {
		assert.False(t, shouldRemovePlaceholder([]*model.PlaceholderValue{
			{Type: model.Placeholder_PlaceholderValue, Value: &types.Value{Kind: &types.Value_StringValue{StringValue: "test"}}},
		}))
	})

	t.Run("shortcut types mean keep", func(t *testing.T) {
		assert.False(t, shouldRemovePlaceholder([]*model.PlaceholderValue{
			{Type: model.Placeholder_PlaceholderToday},
		}))
		assert.False(t, shouldRemovePlaceholder([]*model.PlaceholderValue{
			{Type: model.Placeholder_PlaceholderCurrentUser},
		}))
	})
}

func TestPlaceholderStorageRoundTrip(t *testing.T) {
	t.Run("shortcut roundtrip", func(t *testing.T) {
		// given
		values := []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday}}

		// when
		stored := placeholderValuesToStorage(values)
		outerMap := domain.NewValueMap(map[string]domain.Value{"dueDate": stored}).MapValue()
		restored := storageToPlaceholders(outerMap)

		// then
		require.Len(t, restored, 1)
		assert.Equal(t, "dueDate", restored[0].RelationKey)
		require.Len(t, restored[0].Values, 1)
		assert.Equal(t, model.Placeholder_PlaceholderToday, restored[0].Values[0].Type)
	})

	t.Run("concrete value roundtrip", func(t *testing.T) {
		// given
		values := []*model.PlaceholderValue{
			{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.StringList([]string{"obj1", "obj2"})},
		}

		// when
		stored := placeholderValuesToStorage(values)
		outerMap := domain.NewValueMap(map[string]domain.Value{"assignee": stored}).MapValue()
		restored := storageToPlaceholders(outerMap)

		// then
		require.Len(t, restored, 1)
		require.Len(t, restored[0].Values, 1)
		assert.Equal(t, model.Placeholder_PlaceholderValue, restored[0].Values[0].Type)
		// The roundtrip through domain.Value converts StringList
		restoredStrings := domain.ValueFromProto(restored[0].Values[0].Value).StringList()
		assert.Equal(t, []string{"obj1", "obj2"}, restoredStrings)
	})

	t.Run("combined concrete and shortcut roundtrip", func(t *testing.T) {
		// given
		values := []*model.PlaceholderValue{
			{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.StringList([]string{"obj1"})},
			{Type: model.Placeholder_PlaceholderCurrentUser},
		}

		// when
		stored := placeholderValuesToStorage(values)
		outerMap := domain.NewValueMap(map[string]domain.Value{"assignee": stored}).MapValue()
		restored := storageToPlaceholders(outerMap)

		// then
		require.Len(t, restored, 1)
		require.Len(t, restored[0].Values, 2)

		byType := make(map[model.PlaceholderType]*model.PlaceholderValue)
		for _, v := range restored[0].Values {
			byType[v.Type] = v
		}

		require.Contains(t, byType, model.Placeholder_PlaceholderValue)
		require.Contains(t, byType, model.Placeholder_PlaceholderCurrentUser)
	})
}
