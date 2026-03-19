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
					"type":  domain.Float64(float64(model.Placeholder_PlaceholderValue)),
					"value": domain.Float64(42),
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

	t.Run("returns combined concrete and shortcut placeholder from MapList", func(t *testing.T) {
		// given — stored as [{"type":0,"value":"obj1"},{"type":2}]
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"assignee": domain.MapList([]domain.ValueMap{
					domain.NewValueMap(map[string]domain.Value{
						"type":  domain.Float64(float64(model.Placeholder_PlaceholderValue)),
						"value": domain.String("obj1"),
					}).MapValue(),
					domain.NewValueMap(map[string]domain.Value{
						"type": domain.Float64(float64(model.Placeholder_PlaceholderCurrentUser)),
					}).MapValue(),
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
		assert.Equal(t, model.Placeholder_PlaceholderValue, placeholders[0].Values[0].Type)
		assert.Equal(t, "obj1", placeholders[0].Values[0].Value.GetStringValue())
		assert.Equal(t, model.Placeholder_PlaceholderCurrentUser, placeholders[0].Values[1].Type)
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
		assert.Equal(t, float64(model.Placeholder_PlaceholderToday), inner.GetFloat64("type"))
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
		assert.Equal(t, float64(model.Placeholder_PlaceholderValue), inner.GetFloat64("type"))
		assert.Equal(t, []string{"user1", "user2"}, inner.GetStringList("value"))
	})

	t.Run("sets combined concrete and shortcut as MapList", func(t *testing.T) {
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

		val := m.Get("assignee")
		maps, ok := val.TryMapList()
		require.True(t, ok, "combined entries should be stored as MapList")
		require.Len(t, maps, 2)
		assert.Equal(t, float64(model.Placeholder_PlaceholderValue), maps[0].GetFloat64("type"))
		assert.Equal(t, []string{"obj1"}, maps[0].GetStringList("value"))
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), maps[1].GetFloat64("type"))
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
		assert.Equal(t, float64(model.Placeholder_PlaceholderToday), dueDateInner.GetFloat64("type"))

		assigneeInner, ok := m.TryMapValue("assignee")
		require.True(t, ok)
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), assigneeInner.GetFloat64("type"))
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
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), assigneeInner.GetFloat64("type"))
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
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), dueDateInner.GetFloat64("type"))

		assigneeInner, ok := m.TryMapValue("assignee")
		require.True(t, ok)
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), assigneeInner.GetFloat64("type"))
	})
}

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
		restoredStrings := domain.ValueFromProto(restored[0].Values[0].Value).StringList()
		assert.Equal(t, []string{"obj1", "obj2"}, restoredStrings)
	})

	t.Run("combined concrete and shortcut roundtrip", func(t *testing.T) {
		// given
		values := []*model.PlaceholderValue{
			{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("obj1")},
			{Type: model.Placeholder_PlaceholderCurrentUser},
		}

		// when
		stored := placeholderValuesToStorage(values)
		outerMap := domain.NewValueMap(map[string]domain.Value{"assignee": stored}).MapValue()
		restored := storageToPlaceholders(outerMap)

		// then
		require.Len(t, restored, 1)
		require.Len(t, restored[0].Values, 2)
		// Order is preserved: concrete first, shortcut second
		assert.Equal(t, model.Placeholder_PlaceholderValue, restored[0].Values[0].Type)
		assert.Equal(t, "obj1", restored[0].Values[0].Value.GetStringValue())
		assert.Equal(t, model.Placeholder_PlaceholderCurrentUser, restored[0].Values[1].Type)
	})

	t.Run("single entry stored as MapValue", func(t *testing.T) {
		// given
		values := []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday}}

		// when
		stored := placeholderValuesToStorage(values)

		// then — single entry is a MapValue, not a MapList
		assert.True(t, stored.IsMapValue())
		inner := stored.MapValue()
		assert.Equal(t, float64(model.Placeholder_PlaceholderToday), inner.GetFloat64("type"))
	})

	t.Run("multiple entries stored as MapList", func(t *testing.T) {
		// given
		values := []*model.PlaceholderValue{
			{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("obj1")},
			{Type: model.Placeholder_PlaceholderCurrentUser},
		}

		// when
		stored := placeholderValuesToStorage(values)

		// then — multiple entries are stored as MapList
		assert.True(t, stored.IsMapList())
		maps := stored.MapListValue()
		require.Len(t, maps, 2)
		assert.Equal(t, float64(model.Placeholder_PlaceholderValue), maps[0].GetFloat64("type"))
		assert.Equal(t, "obj1", maps[0].GetString("value"))
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), maps[1].GetFloat64("type"))
	})

	t.Run("proto roundtrip preserves list-of-structs format", func(t *testing.T) {
		// given — simulate full proto roundtrip (store -> proto -> restore)
		values := []*model.PlaceholderValue{
			{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("obj1")},
			{Type: model.Placeholder_PlaceholderCurrentUser},
		}
		stored := placeholderValuesToStorage(values)
		outerMap := domain.NewValueMap(map[string]domain.Value{"assignee": stored}).MapValue()

		// when — convert to proto and back (simulates persistence)
		protoStruct := outerMap.ToProto()
		restoredOuter := domain.ValueFromProto(pbtypes.Struct(protoStruct)).MapValue()
		restored := storageToPlaceholders(restoredOuter)

		// then
		require.Len(t, restored, 1)
		require.Len(t, restored[0].Values, 2)
		assert.Equal(t, model.Placeholder_PlaceholderValue, restored[0].Values[0].Type)
		assert.Equal(t, "obj1", restored[0].Values[0].Value.GetStringValue())
		assert.Equal(t, model.Placeholder_PlaceholderCurrentUser, restored[0].Values[1].Type)
	})
}
