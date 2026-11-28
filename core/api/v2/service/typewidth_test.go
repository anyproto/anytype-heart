package v2service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestTypeWidthCreate(t *testing.T) {
	for _, tc := range []struct {
		name, body, warningPath string
		stored                  float64
		warnings                int
	}{
		{"flat", `{"name":"Journal","layout_width_percent":0.75}`, "/layout_width_percent", 0, 2},
		{"document", `{"kind":"object_type","properties":{"name":"Journal"},"type_settings":{"layout_width_percent":1e400}}`, "/type_settings/layout_width_percent", 1, 1},
		{"legacy", `{"kind":"object_type","properties":{"name":"Journal","layout_width":0.75}}`, "", 0.75, 0},
		{"legacy version", `{"version":2,"kind":"object_type","properties":{"name":"Journal","layout_width":0.75}}`, "", 0.75, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2Fixture(t)
			fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
					value := req.Details.Fields[bundle.RelationKeyLayoutWidth.String()]
					require.NotNil(t, value)
					require.Equal(t, tc.stored, value.GetNumberValue())
					return &pb.RpcObjectCreateObjectTypeResponse{ObjectId: "journal"}
				})
			fx.expectEtagRead("journal")
			result, err := fx.CreateType(context.Background(), testSpaceId, []byte(tc.body), false, false)
			require.NoError(t, err)
			require.Len(t, result.Warnings, tc.warnings)
			for _, warning := range result.Warnings {
				require.Equal(t, tc.warningPath, warning.Path)
			}
		})
	}
}

func TestTypeWidthUpdate(t *testing.T) {
	for _, tc := range []struct {
		name, body, path string
		width            domain.Value
		warnings         int
		mixed            bool
	}{
		{"flat mixed", `{"name":"Wider Chores","layout_width_percent":175.5}`, "/layout_width_percent", domain.Float64(0.755), 0, true},
		{"nested", `{"type_settings":{"layout_width_percent":0.75}}`, "/type_settings/layout_width_percent", domain.Float64(0), 2, false},
		{"flat clamp", `{"layout_width_percent":1e400}`, "/layout_width_percent", domain.Float64(1), 1, false},
		{"clear", `{"type_settings":{"layout_width_percent":null}}`, "/type_settings/layout_width_percent", domain.Null(), 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2Fixture(t)
			fx.addTaskType(t)
			read := editRead(t, `{"formatVersion":"2.0","kind":"object_type","id":"type-chore","internal_key":"chore"}`)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-chore").Return(read, nil)
			fx.mutatorMock.EXPECT().SetTypeDetails(mock.Anything, testSpaceId, "type-chore", mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, _, _ string, updates []domain.Detail, check func([]string) error) error {
					values := domain.NewDetails()
					for _, u := range updates {
						values.Set(u.Key, u.Value)
					}
					require.True(t, tc.width.Equal(values.Get(bundle.RelationKeyLayoutWidth)))
					if tc.mixed {
						require.Equal(t, "Wider Chores", values.GetString(bundle.RelationKeyName))
					}
					return check(read.Heads)
				})
			result, err := fx.UpdateType(context.Background(), testSpaceId, "chore", "", []byte(tc.body), false, false)
			require.NoError(t, err)
			require.Len(t, result.Warnings, tc.warnings)
			for _, warning := range result.Warnings {
				require.Equal(t, tc.path, warning.Path)
			}
		})
	}
}

func TestTypeWidthUpdateRestrictions(t *testing.T) {
	fx := newV2Fixture(t)
	fx.addTaskType(t)
	read := editRead(t, `{"formatVersion":"2.0","kind":"object_type","id":"type-chore","internal_key":"chore"}`)
	body := []byte(`{"name":"Journal","layout_width_percent":175}`)
	read.DetailsRefused = restriction.ErrRestricted
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-chore").Return(read, nil).Twice()
	for _, dry := range []bool{false, true} {
		_, err := fx.UpdateType(context.Background(), testSpaceId, "chore", "", body, dry, false)
		require.Equal(t, v2model.CodeForbidden, v2Err(t, err).Code)
	}
	read.DetailsRefused = nil
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-chore").Return(read, nil)
	fx.mutatorMock.EXPECT().SetTypeDetails(mock.Anything, testSpaceId, "type-chore", mock.Anything, mock.Anything).
		Return(&apicore.ObjectArchivedError{SbType: read.SbType})
	_, err := fx.UpdateType(context.Background(), testSpaceId, "chore", "", body, false, false)
	require.Equal(t, v2model.CodeObjectArchived, v2Err(t, err).Code)
}

func TestTypeWidthOnGenericObject(t *testing.T) {
	for _, tc := range []struct {
		name, initial, op string
		want              any
		changed           bool
	}{
		{"set", "", `{"op":"set_layout_width","layout_width_percent":175}`, 175.0, true},
		{"same", `,"properties":{"layout_width":0.75}`, `{"op":"set_layout_width","layout_width_percent":175}`, 175.0, false},
		{"clear", `,"type_settings":{"layout_width_percent":100}`, `{"op":"set_layout_width","layout_width_percent":null}`, nil, true},
		{"old property write", "", `{"op":"set_properties","set":{"layout_width":0.75}}`, 175.0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2Fixture(t)
			read := editRead(t, `{"formatVersion":"2.0","kind":"object_type","id":"obj1"`+tc.initial+"}")
			st := fx.expectMutate(read)
			result, err := fx.PatchObject(context.Background(), testSpaceId, "obj1", patchBody(tc.op), "", false, false)
			require.NoError(t, err)
			require.Equal(t, tc.changed, result.DiffStats.LayoutWidthChanged)
			body, err := anyblockjson.Marshal(read.SbType, snapshotFromState(*st), anyblockjson.Options{})
			require.NoError(t, err)
			var doc map[string]any
			require.NoError(t, json.Unmarshal(body, &doc))
			require.NotContains(t, doc, "layout_width_percent")
			settings, _ := doc["type_settings"].(map[string]any)
			require.Equal(t, tc.want, settings["layout_width_percent"])
		})
	}
}

func TestTypeWidthReadAndValidate(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		percent any
	}{{"0.75", 175.0}, {"1e-18", nil}} {
		fx := newV2Fixture(t)
		fx.addTaskType(t)
		read := editRead(t, `{"formatVersion":"2.0","kind":"object_type","id":"type-chore","internal_key":"chore","properties":{"layout_width":`+tc.raw+"}}")
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-chore").Return(read, nil)
		body, _, err := fx.GetType(context.Background(), testSpaceId, "chore", ObjectQuery{})
		require.NoError(t, err)
		var result struct {
			Settings map[string]any  `json:"type_settings"`
			Warnings []v2model.Issue `json:"warnings"`
		}
		require.NoError(t, json.Unmarshal(body, &result))
		require.Equal(t, tc.percent, result.Settings["layout_width_percent"])
		if tc.percent == nil {
			require.Contains(t, string(body), "original value")
		}
	}
	fx := newV2Fixture(t)
	report := fx.ValidateDocument([]byte(`{"kind":"object_type","type_settings":{"layout_width_percent":0.75}}`))
	require.Empty(t, report.Issues)
	require.Len(t, report.Warnings, 2)
	require.Equal(t, "/type_settings/layout_width_percent", report.Warnings[0].Path)
}

func TestTypeWidthCombinedPropertyEdit(t *testing.T) {
	const doc = `{"formatVersion":"2.0","kind":"object_type","id":"obj1"}`
	for _, prop := range []string{`"set":{"layout_width":0.75}`, `"set":{"layoutWidth":null}`, `"unset":["Layout width"]`, `"remove":{"layout_width":[]}`} {
		t.Run(prop, func(t *testing.T) {
			fx := newV2Fixture(t)
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:             domain.String("builtin-width"),
				bundle.RelationKeyRelationKey:    domain.String("layoutWidth"),
				bundle.RelationKeyName:           domain.String("Layout width"),
				bundle.RelationKeyRelationFormat: domain.Int64(model.RelationFormat_number),
			})
			read := editRead(t, doc)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)
			_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1", patchBody(`{"op":"set_properties","layout_width_percent":null,`+prop+`}`), "", true, false)
			issue := v2Err(t, err).Issues[0]
			require.Contains(t, issue.Message, "both")
			require.Contains(t, issue.Message, "layout_width_percent")
		})
	}
	t.Run("custom homonym in scope", func(t *testing.T) {
		fx := newV2Fixture(t)
		const custom = "6a7663db61fab21cd4b9e201"
		for _, row := range []struct{ id, key, slug string }{
			{"builtin-width", "layoutWidth", ""}, {"custom-width", custom, "layout_width"},
		} {
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId: domain.String(row.id), bundle.RelationKeyRelationKey: domain.String(row.key),
				bundle.RelationKeyApiObjectKey: domain.String(row.slug), bundle.RelationKeyName: domain.String("Layout width"),
				bundle.RelationKeyRelationFormat: domain.Int64(model.RelationFormat_number),
			})
		}
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId: domain.String("type-for-object"), bundle.RelationKeyUniqueKey: domain.String("ot-objectType"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"custom-width"}),
		})
		read := editRead(t, doc)
		read.Snapshot.Details = pbtypes.EnsureStructInited(read.Snapshot.Details)
		read.Snapshot.Details.Fields[bundle.RelationKeyType.String()] = domain.String("type-for-object").ToProto()
		st := fx.expectMutate(read)
		_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1", patchBody(`{"op":"set_properties","layout_width_percent":175,"set":{"layout_width":42}}`), "", false, false)
		require.NoError(t, err)
		require.Equal(t, 0.75, (*st).Details().Get(bundle.RelationKeyLayoutWidth).Float64())
		require.Equal(t, 42.0, (*st).Details().Get(custom).Float64())
	})
	t.Run("separate ordered operations can intentionally overwrite", func(t *testing.T) {
		fx := newV2Fixture(t)
		st := fx.expectMutate(editRead(t, doc))
		result, err := fx.PatchObject(context.Background(), testSpaceId, "obj1", patchBody(
			`{"op":"set_layout_width","layout_width_percent":175}`,
			`{"op":"set_properties","set":{"layout_width":0.5}}`), "", false, false)
		require.NoError(t, err)
		require.True(t, result.DiffStats.LayoutWidthChanged)
		require.Equal(t, 0.5, (*st).Details().Get(bundle.RelationKeyLayoutWidth).Float64())
	})
}
