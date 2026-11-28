package v2service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestPageWidthCreateValidateAndPatch(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, body string
		width      float64
		warnings   int
	}{
		{"legacy document", `{"version":2,"root":{"fields":{"width":0.75}}}`, 0.75, 0},
		{"shortcut", `{"type":"page","name":"Wide","layout_width_percent":175.5}`, 0.755, 0},
		{"document default type", `{"layout_width_percent":100}`, 0, 0},
		{"fraction", `{"type":"page","layout_width_percent":0.75}`, 0, 2},
		{"large", `{"type":"page","name":"Wide","layout_width_percent":1e400}`, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2Fixture(t)
			snap := fx.expectCreate("newObj")
			fx.expectEtagRead("newObj")
			res, err := fx.CreateObject(ctx, testSpaceId, []byte(tc.body), false, true)
			require.NoError(t, err)
			fields := (*snap).Blocks[0].Fields.GetFields()
			require.Contains(t, fields, "width")
			require.Equal(t, tc.width, fields["width"].GetNumberValue())
			require.Len(t, res.Warnings, tc.warnings)
		})
	}
	for _, tc := range []struct {
		value    string
		before   string
		after    any
		changed  bool
		warnings int
	}{
		{"175.5", "", 175.5, true, 0}, {"100", `,"layout_width_percent":200`, 100.0, true, 0},
		{"null", `,"layout_width_percent":100`, nil, true, 0}, {"null", "", nil, false, 0},
		{"0.75", "", 100.0, true, 2}, {"1e400", "", 200.0, true, 1},
	} {
		t.Run("patch "+tc.value, func(t *testing.T) {
			fx := newV2Fixture(t)
			doc := `{"formatVersion":"2.0","type":"page","id":"obj1","root":{"fields":{"isLocked":false}}` + tc.before + "}"
			captured := fx.expectMutate(editRead(t, doc))
			res, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(`{"op":"set_layout_width","layout_width_percent":`+tc.value+`}`), "", false, true)
			require.NoError(t, err)
			out := stateDoc(t, *captured)
			require.Equal(t, tc.after, out["layout_width_percent"])
			require.Contains(t, (*captured).Pick((*captured).RootId()).Model().Fields.GetFields(), "isLocked")
			require.Equal(t, tc.changed, res.DiffStats.LayoutWidthChanged)
			require.Len(t, res.Warnings, tc.warnings)
			for _, w := range res.Warnings {
				require.Equal(t, "ops[0].layout_width_percent", w.Path)
			}
		})
	}
	fx := newV2Fixture(t)
	report := fx.ValidateDocument([]byte(`{"version":2,"root":{"fields":{"width":0.75}}}`))
	require.Empty(t, report.Issues)
	report = fx.ValidateDocument([]byte(`{"version":1,"root":{"fields":{"width":0.75}}}`))
	require.NotEmpty(t, report.Issues)
	report = fx.ValidateDocument([]byte(`{"layout_width_percent":0.75}`))
	require.Empty(t, report.Issues)
	require.Len(t, report.Warnings, 2)
	for _, v := range []string{"null", `"175"`, "true"} {
		report = fx.ValidateDocument([]byte(`{"layout_width_percent":` + v + `}`))
		require.NotEmpty(t, report.Issues)
	}
	for _, op := range []string{`{"op":"set_layout_width"}`, `{"op":"set_layout_width","layout_width_percent":"175"}`, `{"op":"set_properties","set":{"layout_width":0.75}}`} {
		t.Run(op, func(t *testing.T) {
			fx := newV2Fixture(t)
			fx.expectMutate(editRead(t, editEmptyDoc))
			_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(op), "", false, true)
			issues := v2Err(t, err).Issues
			raw, _ := json.Marshal(issues)
			require.Contains(t, string(raw), "layout_width")
		})
	}
}

// A type's default uses the existing property on the type object itself.
func TestTypeWidthOnObjectEndpoint(t *testing.T) {
	fx := newV2Fixture(t)
	state := fx.expectMutate(editRead(t, `{"formatVersion":"2.0","kind":"object_type","id":"obj1"}`))
	_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
		patchBody(`{"op":"set_properties","set":{"layout_width":0.75}}`), "", false, true)
	require.NoError(t, err)
	require.Equal(t, 0.75, (*state).Details().GetFloat64(bundle.RelationKeyLayoutWidth))
}

func TestGetPageWidth(t *testing.T) {
	for _, tc := range []struct {
		name, members string
		percent       any
		warning       string
	}{
		{"inherit", "", nil, ""},
		{"explicit standard", `,"root":{"fields":{"width":0}}`, 100.0, ""},
		{"legacy detail", `,"properties":{"Layout width":0.75}`, nil, "metadata"},
		{"detail and override", `,"properties":{"Layout width":0.75},"layout_width_percent":100`, 100.0, ""},
		{"detail and legacy override", `,"properties":{"Layout width":0.75},"root":{"fields":{"width":1e-18}}`, nil, "legacy root width"},
		{"legacy precision", `,"root":{"fields":{"width":1e-18}}`, nil, "legacy root width"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2Fixture(t)
			read := editRead(t, `{"id":"obj1","type":"page","formatVersion":"2.0"`+tc.members+`}`)
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)
			body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", ObjectQuery{})
			require.NoError(t, err)
			var doc map[string]any
			require.NoError(t, json.Unmarshal(body, &doc))
			require.Equal(t, tc.percent, doc["layout_width_percent"])
			if tc.warning != "metadata" {
				require.NotContains(t, string(body), "This property is metadata")
			}
			if tc.warning != "" {
				require.Contains(t, string(body), tc.warning)
			}
		})
	}
}
