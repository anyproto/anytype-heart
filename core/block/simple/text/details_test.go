package text

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testBlock = &model.Block{
	Id: "db",
	Fields: &types.Struct{
		Fields: map[string]*types.Value{
			DetailsKeyFieldName: pbtypes.StringList([]string{"title", "checked", "align"}),
		},
	},
	Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{},
	},
}

func TestNewDetails(t *testing.T) {
	b := simple.New(testBlock)
	assert.Implements(t, (*DetailsBlock)(nil), b)
}

func TestTextDetails_DetailsInit(t *testing.T) {
	db := simple.New(testBlock).(DetailsBlock)
	db.DetailsInit(&testDetailsService{Struct: &types.Struct{
		Fields: map[string]*types.Value{
			"title": pbtypes.String("titleFromDetails"),
		},
	}})
	assert.Equal(t, "titleFromDetails", db.GetText())
}

func TestTextDetails_OnDetailsChange(t *testing.T) {
	ds := &testDetailsService{Struct: &types.Struct{
		Fields: map[string]*types.Value{
			"title": pbtypes.String("titleFromDetails"),
		},
	}}
	orig := simple.New(testBlock).(DetailsBlock)
	db := orig.Copy().(DetailsBlock)
	db.DetailsInit(ds)
	assert.Equal(t, "titleFromDetails", db.GetText())
	ds.Details().Fields["checked"] = pbtypes.Bool(true)
	ds.Details().Fields["align"] = pbtypes.Int64(int64(model.Block_AlignRight))
	ds.Details().Fields["title"] = pbtypes.String("changed")

	msgs, err := db.OnDetailsChange(orig, ds)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
}

func TestTextDetails_DetailsApply(t *testing.T) {
	orig := simple.New(testBlock).(DetailsBlock)
	db := orig.Copy().(DetailsBlock)
	ds := &testDetailsService{Struct: &types.Struct{
		Fields: map[string]*types.Value{
			"title": pbtypes.String("titleFromDetails"),
		},
	}}
	db.DetailsInit(ds)
	require.NoError(t, db.SetText("changed", nil))
	db.SetChecked(true)
	db.Model().Align = model.Block_AlignRight
	msgs, err := db.ApplyToDetails(orig, ds)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	st := msgs[0].Msg.GetBlockSetText()
	require.NotNil(t, st)
	require.NotNil(t, st.Text)
	require.NotNil(t, st.Checked)
	assert.Equal(t, "changed", st.Text.Value)
	assert.Equal(t, true, st.Checked.Value)
	msgs, err = db.Diff(orig)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Nil(t, msgs[0].Msg.GetBlockSetText().GetText())
	assert.Nil(t, msgs[0].Msg.GetBlockSetText().GetChecked())
	assert.Equal(t, "changed", pbtypes.GetString(ds.Details(), "title"))
	assert.Equal(t, true, pbtypes.GetBool(ds.Details(), "checked"))
	assert.Equal(t, int64(model.Block_AlignRight), pbtypes.GetInt64(ds.Details(), "align"))

}

type testDetailsService struct {
	*types.Struct
}

func (t *testDetailsService) Details() *types.Struct {
	return t.Struct
}

func (t *testDetailsService) SetDetail(key string, value *types.Value) {
	if t.Struct == nil || t.Struct.Fields == nil {
		t.Struct = &types.Struct{
			Fields: map[string]*types.Value{},
		}
	}
	t.Struct.Fields[key] = value
}
