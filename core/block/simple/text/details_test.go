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
			DetailsKeyFieldName: pbtypes.StringList([]string{"title", "checked"}),
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
	db := simple.New(testBlock).Copy().(DetailsBlock)
	db.DetailsInit(&testDetailsService{Struct: &types.Struct{
		Fields: map[string]*types.Value{
			"title": pbtypes.String("titleFromDetails"),
		},
	}})
	assert.Equal(t, "titleFromDetails", db.GetText())
}

func TestTextDetails_ApplyToDetails(t *testing.T) {
	orig := simple.New(testBlock).Copy().(DetailsBlock)
	db := orig.Copy().(DetailsBlock)
	ds := &testDetailsService{Struct: &types.Struct{
		Fields: map[string]*types.Value{
			"title": pbtypes.String("titleFromDetails"),
		},
	}}
	db.DetailsInit(ds)
	require.NoError(t, db.SetText("changed", nil))
	ok, err := db.ApplyToDetails(orig, ds)
	require.NoError(t, err)
	assert.True(t, ok)
	orig.SetText("changed", nil)
	ok, err = db.ApplyToDetails(orig, ds)
	require.NoError(t, err)
	assert.False(t, ok)
	db.SetChecked(true)
	ok, err = db.ApplyToDetails(orig, ds)
	require.NoError(t, err)
	assert.True(t, ok)
	orig.SetChecked(true)
	ok, err = db.ApplyToDetails(orig, ds)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestTextDetails_Diff(t *testing.T) {
	orig := simple.New(testBlock).Copy().(DetailsBlock)
	db := orig.Copy().(DetailsBlock)
	ds := &testDetailsService{Struct: &types.Struct{
		Fields: map[string]*types.Value{
			"title": pbtypes.String("titleFromDetails"),
		},
	}}
	db.DetailsInit(ds)
	require.NoError(t, db.SetText("changed", nil))
	db.SetChecked(true)
	ok, err := db.ApplyToDetails(orig, ds)
	require.NoError(t, err)
	require.True(t, ok)

	assert.Equal(t, "changed", pbtypes.GetString(ds.Struct, "title"))
	assert.Equal(t, true, pbtypes.GetBool(ds.Struct, "checked"))

	msgs, err := orig.Diff(db)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.True(t, msgs[0].Virtual)
	setText := msgs[0].Msg.GetBlockSetText()
	require.NotNil(t, setText)
	require.NotNil(t, setText.Text)
	assert.Equal(t, "changed", setText.Text.Value)
	require.NotNil(t, setText.Checked)
	assert.Equal(t, true, setText.Checked.Value)
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
