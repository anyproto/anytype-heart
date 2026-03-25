package templateimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils/mock_relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// newFormatFetcher creates a mock RelationFormatFetcher. The formats map specifies
// the expected format for each relation key. Keys not in the map default to longtext.
func newFormatFetcher(t *testing.T, formats map[string]model.RelationFormat) *mock_relationutils.MockRelationFormatFetcher {
	ff := mock_relationutils.NewMockRelationFormatFetcher(t)
	ff.EXPECT().GetRelationFormatByKey(mock.Anything, mock.Anything).RunAndReturn(
		func(_ string, key domain.RelationKey) (model.RelationFormat, error) {
			if formats != nil {
				if f, ok := formats[key.String()]; ok {
					return f, nil
				}
			}
			return model.RelationFormat_longtext, nil
		},
	).Maybe()
	return ff
}

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
				"priority": domain.MapList([]domain.ValueMap{
					domain.NewValueMap(map[string]domain.Value{
						"type":  domain.Float64(float64(model.Placeholder_PlaceholderValue)),
						"value": domain.Float64(42),
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
	const (
		templateId  = "template1"
		testSpaceId = "space1"
	)

	t.Run("sets shortcut placeholder on template", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"dueDate": model.RelationFormat_date,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday}}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		maps, ok := m.Get("dueDate").TryMapList()
		require.True(t, ok)
		require.Len(t, maps, 1)
		assert.Equal(t, float64(model.Placeholder_PlaceholderToday), maps[0].GetFloat64("type"))
	})

	t.Run("sets concrete value placeholder", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"assignee": model.RelationFormat_object,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("user1")},
			}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		maps, ok := m.Get("assignee").TryMapList()
		require.True(t, ok)
		require.Len(t, maps, 1)
		assert.Equal(t, float64(model.Placeholder_PlaceholderValue), maps[0].GetFloat64("type"))
		assert.Equal(t, "user1", maps[0].GetString("value"))
	})

	t.Run("sets combined concrete and shortcut as MapList", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"assignee": model.RelationFormat_object,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("obj1")},
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
		assert.Equal(t, "obj1", maps[0].GetString("value"))
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), maps[1].GetFloat64("type"))
	})

	t.Run("sets multiple relation placeholders", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"dueDate":  model.RelationFormat_date,
			"assignee": model.RelationFormat_object,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday}}},
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser}}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)

		dueDateMaps, ok := m.Get("dueDate").TryMapList()
		require.True(t, ok)
		require.Len(t, dueDateMaps, 1)
		assert.Equal(t, float64(model.Placeholder_PlaceholderToday), dueDateMaps[0].GetFloat64("type"))

		assigneeMaps, ok := m.Get("assignee").TryMapList()
		require.True(t, ok)
		require.Len(t, assigneeMaps, 1)
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), assigneeMaps[0].GetFloat64("type"))
	})

	t.Run("empty values are skipped without modifying existing", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"dueDate": storageShortcut(model.Placeholder_PlaceholderToday),
			})},
		}, false)
		ff := newFormatFetcher(t, nil)
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate"},
		})

		// then
		require.NoError(t, err)
		assert.True(t, tmpl.NewState().Details().Has(bundle.RelationKeyTemplatePlaceholders),
			"existing placeholder should be preserved when empty values are skipped")
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
		tmpl.SetSpaceId(testSpaceId)
		_ = tmpl.SetDetails(nil, []domain.Detail{
			{Key: bundle.RelationKeyTemplatePlaceholders, Value: domain.NewValueMap(map[string]domain.Value{
				"dueDate":  storageShortcut(model.Placeholder_PlaceholderToday),
				"assignee": storageShortcut(model.Placeholder_PlaceholderCurrentUser),
			})},
		}, false)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"dueDate": model.RelationFormat_object,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "dueDate", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser}}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)

		dueDateMaps, ok := m.Get("dueDate").TryMapList()
		require.True(t, ok, "overwritten entry should be stored as MapList")
		require.Len(t, dueDateMaps, 1)
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), dueDateMaps[0].GetFloat64("type"))

		// assignee was not overwritten, so it stays in its original format
		assigneeMaps := m.Get("assignee").MapListValue()
		require.Len(t, assigneeMaps, 1)
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), assigneeMaps[0].GetFloat64("type"))
	})
}

func TestService_SetTemplatePlaceholders_Validation(t *testing.T) {
	const (
		templateId  = "template1"
		testSpaceId = "space1"
	)

	t.Run("rejects today placeholder on non-date format", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"name": model.RelationFormat_shorttext,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "name", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderToday}}},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "date is expected")
	})

	t.Run("rejects current user placeholder on non-object format", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"priority": model.RelationFormat_number,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "priority", Values: []*model.PlaceholderValue{{Type: model.Placeholder_PlaceholderCurrentUser}}},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "object is expected")
	})

	t.Run("rejects multiple values for non-object format", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"priority": model.RelationFormat_number,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "priority", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.Float64(1)},
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.Float64(2)},
			}},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot handle multiple values")
	})

	t.Run("rejects string value for number format", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"priority": model.RelationFormat_number,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "priority", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("not a number")},
			}},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "should be number")
	})

	t.Run("rejects number value for string format", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"name": model.RelationFormat_shorttext,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "name", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.Float64(42)},
			}},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "should be string")
	})

	t.Run("rejects string value for checkbox format", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"done": model.RelationFormat_checkbox,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "done", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("true")},
			}},
		})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "should be boolean")
	})

	t.Run("accepts valid concrete values per format", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"name":     model.RelationFormat_shorttext,
			"priority": model.RelationFormat_number,
			"done":     model.RelationFormat_checkbox,
			"assignee": model.RelationFormat_object,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "name", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("hello")},
			}},
			{RelationKey: "priority", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.Float64(42)},
			}},
			{RelationKey: "done", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.Bool(true)},
			}},
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("user1")},
			}},
		})

		// then
		require.NoError(t, err)

		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		assert.NotNil(t, m.Get("name").MapListValue())
		assert.NotNil(t, m.Get("priority").MapListValue())
		assert.NotNil(t, m.Get("done").MapListValue())
		assert.NotNil(t, m.Get("assignee").MapListValue())
	})

	t.Run("allows multiple values for object format", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"assignee": model.RelationFormat_object,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "assignee", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("user1")},
				{Type: model.Placeholder_PlaceholderCurrentUser},
			}},
		})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		maps := m.Get("assignee").MapListValue()
		require.Len(t, maps, 2)
	})

	t.Run("allows multiple values for tag format", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String()).(*smarttest.SmartTest)
		tmpl.SetSpaceId(testSpaceId)
		ff := newFormatFetcher(t, map[string]model.RelationFormat{
			"tags": model.RelationFormat_tag,
		})
		s := service{
			picker:        &testPicker{sb: tmpl},
			store:         objectstore.NewStoreFixture(t),
			formatFetcher: ff,
		}

		// when
		err := s.SetTemplatePlaceholders(nil, templateId, []*model.Placeholder{
			{RelationKey: "tags", Values: []*model.PlaceholderValue{
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("tag1")},
				{Type: model.Placeholder_PlaceholderValue, Value: pbtypes.String("tag2")},
			}},
		})

		// then
		require.NoError(t, err)
	})
}

func TestService_DeleteTemplatePlaceholders(t *testing.T) {
	const templateId = "template1"

	t.Run("deletes one placeholder and preserves others", func(t *testing.T) {
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
		err := s.DeleteTemplatePlaceholders(nil, templateId, []string{"dueDate"})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		assert.True(t, m.Get("dueDate").IsEmpty(), "dueDate should be removed")

		assigneeMaps := m.Get("assignee").MapListValue()
		require.Len(t, assigneeMaps, 1)
		assert.Equal(t, float64(model.Placeholder_PlaceholderCurrentUser), assigneeMaps[0].GetFloat64("type"))
	})

	t.Run("deletes last placeholder removes detail entirely", func(t *testing.T) {
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
		err := s.DeleteTemplatePlaceholders(nil, templateId, []string{"dueDate"})

		// then
		require.NoError(t, err)
		assert.False(t, tmpl.NewState().Details().Has(bundle.RelationKeyTemplatePlaceholders))
	})

	t.Run("deletes multiple keys at once", func(t *testing.T) {
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
		err := s.DeleteTemplatePlaceholders(nil, templateId, []string{"dueDate", "assignee"})

		// then
		require.NoError(t, err)
		assert.False(t, tmpl.NewState().Details().Has(bundle.RelationKeyTemplatePlaceholders))
	})

	t.Run("deleting non-existent key does nothing", func(t *testing.T) {
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
		err := s.DeleteTemplatePlaceholders(nil, templateId, []string{"nonExistent"})

		// then
		require.NoError(t, err)
		m := tmpl.NewState().Details().GetMapValue(bundle.RelationKeyTemplatePlaceholders)
		dueDateMaps := m.Get("dueDate").MapListValue()
		require.Len(t, dueDateMaps, 1, "existing placeholder should be preserved")
	})

	t.Run("no existing placeholders returns no error", func(t *testing.T) {
		// given
		tmpl := newTemplateTest(templateId, bundle.TypeKeyTask.String())
		s := service{
			picker: &testPicker{sb: tmpl},
			store:  objectstore.NewStoreFixture(t),
		}

		// when
		err := s.DeleteTemplatePlaceholders(nil, templateId, []string{"dueDate"})

		// then
		require.NoError(t, err)
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
		err := s.DeleteTemplatePlaceholders(nil, templateId, []string{"dueDate"})

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "object is not a template")
	})
}
