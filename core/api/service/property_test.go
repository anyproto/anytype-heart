package service

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func stringPtr(s string) *string          { return &s }
func boolPtr(b bool) *bool                { return &b }
func stringSlicePtr(s []string) *[]string { return &s }

func TestProcessProperties(t *testing.T) {
	ctx := context.Background()

	setupPropertyMapMock := func(fx *fixture) {
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectSearchRequest) bool {
			return req.SpaceId == mockedSpaceId &&
				req.Filters[0].RelationKey == bundle.RelationKeyResolvedLayout.String() &&
				req.Filters[0].Value.Equal(pbtypes.Int64(int64(model.ObjectType_relation)))
		})).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("text_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("text_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("text_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("Text Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_longtext)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("number_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("number_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("number_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("Number Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_number)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("select_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("select_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("select_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("Select Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_status)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("multi_select_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("multi_select_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("multi_select_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("Multi Select Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_tag)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("date_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("date_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("date_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("Date Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_date)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("files_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("files_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("files_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("Files Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_file)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("checkbox_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("checkbox_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("checkbox_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("Checkbox Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_checkbox)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("url_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("url_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("url_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("URL Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_url)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("email_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("email_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("email_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("Email Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_email)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("phone_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("phone_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("phone_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("Phone Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_phone)),
					},
				},
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String("objects_prop_id"),
						bundle.RelationKeyRelationKey.String():    pbtypes.String("objects_prop"),
						bundle.RelationKeyApiObjectKey.String():   pbtypes.String("objects_prop"),
						bundle.RelationKeyName.String():           pbtypes.String("Objects Property"),
						bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(model.RelationFormat_object)),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Maybe()
	}

	setupValidationMock := func(fx *fixture, objectId string, isValid bool, layout model.ObjectTypeLayout) {
		response := &pb.RpcObjectSearchResponse{
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}
		if isValid {
			response.Records = []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyResolvedLayout.String(): pbtypes.Int64(int64(layout)),
					},
				},
			}
		}
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(objectId),
				},
			},
			Keys: []string{bundle.RelationKeyResolvedLayout.String()},
		}).Return(response).Maybe()
	}

	setupTagValidationMock := func(fx *fixture, tagId string, propertyKey string, isValid bool) {
		response := &pb.RpcObjectSearchResponse{
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}
		if isValid {
			response.Records = []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyResolvedLayout.String(): pbtypes.Int64(int64(model.ObjectType_tag)),
						bundle.RelationKeyRelationKey.String():    pbtypes.String(propertyKey),
					},
				},
			}
		}
		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyId.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(tagId),
				},
			},
			Keys: []string{bundle.RelationKeyResolvedLayout.String(), bundle.RelationKeyRelationKey.String()},
		}).Return(response).Maybe()
	}

	t.Run("empty entries", func(t *testing.T) {
		fx := newFixture(t)
		result, err := fx.service.processProperties(ctx, mockedSpaceId, []apimodel.PropertyLinkWithValue{})
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("text property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("valid value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.TextPropertyLinkValue{
					PropertyKey: "text_prop",
					Text:        stringPtr("Hello World"),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "Hello World", result["text_prop"].GetStringValue())
		})

		t.Run("empty string", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.TextPropertyLinkValue{
					PropertyKey: "text_prop",
					Text:        stringPtr(""),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "", result["text_prop"].GetStringValue())
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.TextPropertyLinkValue{
					PropertyKey: "text_prop",
					Text:        nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["text_prop"].GetNullValue())
		})

		t.Run("whitespace trimmed", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.TextPropertyLinkValue{
					PropertyKey: "text_prop",
					Text:        stringPtr("  Hello World  "),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "Hello World", result["text_prop"].GetStringValue())
		})
	})

	t.Run("number property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("valid value", func(t *testing.T) {
			num := 42.5
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.NumberPropertyLinkValue{
					PropertyKey: "number_prop",
					Number:      &num,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, 42.5, result["number_prop"].GetNumberValue())
		})

		t.Run("zero value", func(t *testing.T) {
			zero := 0.0
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.NumberPropertyLinkValue{
					PropertyKey: "number_prop",
					Number:      &zero,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, 0.0, result["number_prop"].GetNumberValue())
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.NumberPropertyLinkValue{
					PropertyKey: "number_prop",
					Number:      nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["number_prop"].GetNullValue())
		})

		t.Run("negative value", func(t *testing.T) {
			neg := -123.45
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.NumberPropertyLinkValue{
					PropertyKey: "number_prop",
					Number:      &neg,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, -123.45, result["number_prop"].GetNumberValue())
		})
	})

	t.Run("select property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("valid value", func(t *testing.T) {
			setupTagValidationMock(fx, "tag123", "select_prop", true)
			tagId := "tag123"
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.SelectPropertyLinkValue{
					PropertyKey: "select_prop",
					Select:      &tagId,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "tag123", result["select_prop"].GetStringValue())
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.SelectPropertyLinkValue{
					PropertyKey: "select_prop",
					Select:      nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["select_prop"].GetNullValue())
		})

		t.Run("invalid tag", func(t *testing.T) {
			setupTagValidationMock(fx, "invalid_tag", "select_prop", false)
			tagId := "invalid_tag"
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.SelectPropertyLinkValue{
					PropertyKey: "select_prop",
					Select:      &tagId,
				}},
			}
			_, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid select option")
		})

		t.Run("empty string", func(t *testing.T) {
			setupTagValidationMock(fx, "", "select_prop", false)
			tagId := ""
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.SelectPropertyLinkValue{
					PropertyKey: "select_prop",
					Select:      &tagId,
				}},
			}
			_, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid select option")
		})
	})

	t.Run("multi_select property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("valid values", func(t *testing.T) {
			setupTagValidationMock(fx, "tag1", "multi_select_prop", true)
			setupTagValidationMock(fx, "tag2", "multi_select_prop", true)
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.MultiSelectPropertyLinkValue{
					PropertyKey: "multi_select_prop",
					MultiSelect: stringSlicePtr([]string{"tag1", "tag2"}),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			list := result["multi_select_prop"].GetListValue()
			require.NotNil(t, list)
			assert.Len(t, list.Values, 2)
			assert.Equal(t, "tag1", list.Values[0].GetStringValue())
			assert.Equal(t, "tag2", list.Values[1].GetStringValue())
		})

		t.Run("empty array", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.MultiSelectPropertyLinkValue{
					PropertyKey: "multi_select_prop",
					MultiSelect: stringSlicePtr([]string{}),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["multi_select_prop"].GetNullValue())
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.MultiSelectPropertyLinkValue{
					PropertyKey: "multi_select_prop",
					MultiSelect: nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["multi_select_prop"].GetNullValue())
		})

		t.Run("invalid tag", func(t *testing.T) {
			setupTagValidationMock(fx, "invalid_tag", "multi_select_prop", false)
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.MultiSelectPropertyLinkValue{
					PropertyKey: "multi_select_prop",
					MultiSelect: stringSlicePtr([]string{"invalid_tag"}),
				}},
			}
			_, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid multi_select option")
		})

		t.Run("single value", func(t *testing.T) {
			setupTagValidationMock(fx, "tag1", "multi_select_prop", true)
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.MultiSelectPropertyLinkValue{
					PropertyKey: "multi_select_prop",
					MultiSelect: stringSlicePtr([]string{"tag1"}),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			list := result["multi_select_prop"].GetListValue()
			require.NotNil(t, list)
			assert.Len(t, list.Values, 1)
			assert.Equal(t, "tag1", list.Values[0].GetStringValue())
		})
	})

	t.Run("date property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("valid RFC3339 date", func(t *testing.T) {
			dateStr := "2023-12-25T10:30:00Z"
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.DatePropertyLinkValue{
					PropertyKey: "date_prop",
					Date:        &dateStr,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Greater(t, result["date_prop"].GetNumberValue(), float64(0))
		})

		t.Run("valid date only format", func(t *testing.T) {
			dateStr := "2023-12-25"
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.DatePropertyLinkValue{
					PropertyKey: "date_prop",
					Date:        &dateStr,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Greater(t, result["date_prop"].GetNumberValue(), float64(0))
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.DatePropertyLinkValue{
					PropertyKey: "date_prop",
					Date:        nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["date_prop"].GetNullValue())
		})

		t.Run("invalid format", func(t *testing.T) {
			dateStr := "invalid-date"
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.DatePropertyLinkValue{
					PropertyKey: "date_prop",
					Date:        &dateStr,
				}},
			}
			_, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid date format")
		})

		t.Run("empty string", func(t *testing.T) {
			dateStr := ""
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.DatePropertyLinkValue{
					PropertyKey: "date_prop",
					Date:        &dateStr,
				}},
			}
			_, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid date format")
		})
	})

	t.Run("files property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("valid file IDs", func(t *testing.T) {
			setupValidationMock(fx, "file1", true, model.ObjectType_file)
			setupValidationMock(fx, "file2", true, model.ObjectType_file)
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.FilesPropertyLinkValue{
					PropertyKey: "files_prop",
					Files:       stringSlicePtr([]string{"file1", "file2"}),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			list := result["files_prop"].GetListValue()
			require.NotNil(t, list)
			assert.Len(t, list.Values, 2)
			assert.Equal(t, "file1", list.Values[0].GetStringValue())
			assert.Equal(t, "file2", list.Values[1].GetStringValue())
		})

		t.Run("empty array", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.FilesPropertyLinkValue{
					PropertyKey: "files_prop",
					Files:       stringSlicePtr([]string{}),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["files_prop"].GetNullValue())
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.FilesPropertyLinkValue{
					PropertyKey: "files_prop",
					Files:       nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["files_prop"].GetNullValue())
		})

		t.Run("invalid file reference", func(t *testing.T) {
			setupValidationMock(fx, "invalid_file", false, model.ObjectType_file)
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.FilesPropertyLinkValue{
					PropertyKey: "files_prop",
					Files:       stringSlicePtr([]string{"invalid_file"}),
				}},
			}
			_, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid file reference")
		})

		t.Run("single file", func(t *testing.T) {
			setupValidationMock(fx, "file1", true, model.ObjectType_file)
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.FilesPropertyLinkValue{
					PropertyKey: "files_prop",
					Files:       stringSlicePtr([]string{"file1"}),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			list := result["files_prop"].GetListValue()
			require.NotNil(t, list)
			assert.Len(t, list.Values, 1)
			assert.Equal(t, "file1", list.Values[0].GetStringValue())
		})
	})

	t.Run("checkbox property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("true value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.CheckboxPropertyLinkValue{
					PropertyKey: "checkbox_prop",
					Checkbox:    boolPtr(true),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.True(t, result["checkbox_prop"].GetBoolValue())
		})

		t.Run("false value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.CheckboxPropertyLinkValue{
					PropertyKey: "checkbox_prop",
					Checkbox:    boolPtr(false),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.False(t, result["checkbox_prop"].GetBoolValue())
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.CheckboxPropertyLinkValue{
					PropertyKey: "checkbox_prop",
					Checkbox:    nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["checkbox_prop"].GetNullValue())
		})
	})

	t.Run("url property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("valid value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.URLPropertyLinkValue{
					PropertyKey: "url_prop",
					Url:         stringPtr("https://example.com"),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "https://example.com", result["url_prop"].GetStringValue())
		})

		t.Run("empty string", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.URLPropertyLinkValue{
					PropertyKey: "url_prop",
					Url:         stringPtr(""),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "", result["url_prop"].GetStringValue())
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.URLPropertyLinkValue{
					PropertyKey: "url_prop",
					Url:         nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["url_prop"].GetNullValue())
		})

		t.Run("whitespace trimmed", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.URLPropertyLinkValue{
					PropertyKey: "url_prop",
					Url:         stringPtr("  https://example.com  "),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "https://example.com", result["url_prop"].GetStringValue())
		})
	})

	t.Run("email property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("valid value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.EmailPropertyLinkValue{
					PropertyKey: "email_prop",
					Email:       stringPtr("test@example.com"),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "test@example.com", result["email_prop"].GetStringValue())
		})

		t.Run("empty string", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.EmailPropertyLinkValue{
					PropertyKey: "email_prop",
					Email:       stringPtr(""),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "", result["email_prop"].GetStringValue())
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.EmailPropertyLinkValue{
					PropertyKey: "email_prop",
					Email:       nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["email_prop"].GetNullValue())
		})

		t.Run("whitespace trimmed", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.EmailPropertyLinkValue{
					PropertyKey: "email_prop",
					Email:       stringPtr("  test@example.com  "),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "test@example.com", result["email_prop"].GetStringValue())
		})
	})

	t.Run("phone property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("valid value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.PhonePropertyLinkValue{
					PropertyKey: "phone_prop",
					Phone:       stringPtr("+1234567890"),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "+1234567890", result["phone_prop"].GetStringValue())
		})

		t.Run("empty string", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.PhonePropertyLinkValue{
					PropertyKey: "phone_prop",
					Phone:       stringPtr(""),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "", result["phone_prop"].GetStringValue())
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.PhonePropertyLinkValue{
					PropertyKey: "phone_prop",
					Phone:       nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["phone_prop"].GetNullValue())
		})

		t.Run("whitespace trimmed", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.PhonePropertyLinkValue{
					PropertyKey: "phone_prop",
					Phone:       stringPtr("  +1234567890  "),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, "+1234567890", result["phone_prop"].GetStringValue())
		})
	})

	t.Run("objects property", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("valid object IDs", func(t *testing.T) {
			setupValidationMock(fx, "obj1", true, model.ObjectType_basic)
			setupValidationMock(fx, "obj2", true, model.ObjectType_basic)
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.ObjectsPropertyLinkValue{
					PropertyKey: "objects_prop",
					Objects:     stringSlicePtr([]string{"obj1", "obj2"}),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			list := result["objects_prop"].GetListValue()
			require.NotNil(t, list)
			assert.Len(t, list.Values, 2)
			assert.Equal(t, "obj1", list.Values[0].GetStringValue())
			assert.Equal(t, "obj2", list.Values[1].GetStringValue())
		})

		t.Run("empty array", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.ObjectsPropertyLinkValue{
					PropertyKey: "objects_prop",
					Objects:     stringSlicePtr([]string{}),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["objects_prop"].GetNullValue())
		})

		t.Run("nil value", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.ObjectsPropertyLinkValue{
					PropertyKey: "objects_prop",
					Objects:     nil,
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			assert.Equal(t, types.NullValue_NULL_VALUE, result["objects_prop"].GetNullValue())
		})

		t.Run("invalid object reference", func(t *testing.T) {
			setupValidationMock(fx, "invalid_obj", false, model.ObjectType_basic)
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.ObjectsPropertyLinkValue{
					PropertyKey: "objects_prop",
					Objects:     stringSlicePtr([]string{"invalid_obj"}),
				}},
			}
			_, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid object reference")
		})

		t.Run("single object", func(t *testing.T) {
			setupValidationMock(fx, "obj1", true, model.ObjectType_basic)
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.ObjectsPropertyLinkValue{
					PropertyKey: "objects_prop",
					Objects:     stringSlicePtr([]string{"obj1"}),
				}},
			}
			result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.NoError(t, err)
			list := result["objects_prop"].GetListValue()
			require.NotNil(t, list)
			assert.Len(t, list.Values, 1)
			assert.Equal(t, "obj1", list.Values[0].GetStringValue())
		})
	})

	t.Run("error cases", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		t.Run("unknown property key", func(t *testing.T) {
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.TextPropertyLinkValue{
					PropertyKey: "unknown_prop",
					Text:        stringPtr("value"),
				}},
			}
			_, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown property key")
		})

		t.Run("excluded system property", func(t *testing.T) {
			fx := newFixture(t)
			setupPropertyMapMock(fx)
			entries := []apimodel.PropertyLinkWithValue{
				{WrappedPropertyLinkWithValue: apimodel.TextPropertyLinkValue{
					PropertyKey: "id",
					Text:        stringPtr("some_id"),
				}},
			}
			_, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot be set directly as it is a reserved system property")
		})
	})

	t.Run("multiple properties", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)
		setupValidationMock(fx, "file1", true, model.ObjectType_file)
		setupTagValidationMock(fx, "tag1", "select_prop", true)

		num := 42.0
		entries := []apimodel.PropertyLinkWithValue{
			{WrappedPropertyLinkWithValue: apimodel.TextPropertyLinkValue{
				PropertyKey: "text_prop",
				Text:        stringPtr("Hello"),
			}},
			{WrappedPropertyLinkWithValue: apimodel.NumberPropertyLinkValue{
				PropertyKey: "number_prop",
				Number:      &num,
			}},
			{WrappedPropertyLinkWithValue: apimodel.SelectPropertyLinkValue{
				PropertyKey: "select_prop",
				Select:      stringPtr("tag1"),
			}},
			{WrappedPropertyLinkWithValue: apimodel.FilesPropertyLinkValue{
				PropertyKey: "files_prop",
				Files:       stringSlicePtr([]string{"file1"}),
			}},
			{WrappedPropertyLinkWithValue: apimodel.CheckboxPropertyLinkValue{
				PropertyKey: "checkbox_prop",
				Checkbox:    boolPtr(true),
			}},
		}

		result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
		require.NoError(t, err)
		assert.Len(t, result, 5)
		assert.Equal(t, "Hello", result["text_prop"].GetStringValue())
		assert.Equal(t, 42.0, result["number_prop"].GetNumberValue())
		assert.Equal(t, "tag1", result["select_prop"].GetStringValue())
		assert.Equal(t, "file1", result["files_prop"].GetListValue().Values[0].GetStringValue())
		assert.True(t, result["checkbox_prop"].GetBoolValue())
	})

	t.Run("partial update", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		entries := []apimodel.PropertyLinkWithValue{
			{WrappedPropertyLinkWithValue: apimodel.TextPropertyLinkValue{
				PropertyKey: "text_prop",
				Text:        stringPtr("New value"),
			}},
		}

		result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Contains(t, result, "text_prop")
		assert.NotContains(t, result, "number_prop")
		assert.NotContains(t, result, "checkbox_prop")
	})

	t.Run("clearing values", func(t *testing.T) {
		fx := newFixture(t)
		setupPropertyMapMock(fx)

		entries := []apimodel.PropertyLinkWithValue{
			{WrappedPropertyLinkWithValue: apimodel.TextPropertyLinkValue{
				PropertyKey: "text_prop",
				Text:        stringPtr(""),
			}},
			{WrappedPropertyLinkWithValue: apimodel.NumberPropertyLinkValue{
				PropertyKey: "number_prop",
				Number:      nil,
			}},
			{WrappedPropertyLinkWithValue: apimodel.SelectPropertyLinkValue{
				PropertyKey: "select_prop",
				Select:      nil,
			}},
			{WrappedPropertyLinkWithValue: apimodel.FilesPropertyLinkValue{
				PropertyKey: "files_prop",
				Files:       stringSlicePtr([]string{}),
			}},
		}

		result, err := fx.service.processProperties(ctx, mockedSpaceId, entries)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result), 4)
		assert.Equal(t, "", result["text_prop"].GetStringValue())
		assert.Equal(t, types.NullValue_NULL_VALUE, result["number_prop"].GetNullValue())
		assert.Equal(t, types.NullValue_NULL_VALUE, result["select_prop"].GetNullValue())
		assert.Equal(t, types.NullValue_NULL_VALUE, result["files_prop"].GetNullValue())
	})
}
