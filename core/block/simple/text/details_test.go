package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	types "google.golang.org/protobuf/types/known/structpb"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	db.DetailsInit(&testDetailsService{details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"title": domain.String("titleFromDetails"),
	}),
	})
	assert.Equal(t, "titleFromDetails", db.GetText())
}

func TestTextDetails_DetailsInit_DoNotChangeCheckedStateIfNotPresent(t *testing.T) {
	db := simple.New(testBlock).Copy().(DetailsBlock)
	db.SetChecked(true)
	db.DetailsInit(&testDetailsService{details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})})
	assert.Equal(t, db.GetChecked(), true)
}

func TestTextDetails_ApplyToDetails(t *testing.T) {
	orig := simple.New(testBlock).Copy().(DetailsBlock)
	db := orig.Copy().(DetailsBlock)
	ds := &testDetailsService{details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		"title": domain.String("titleFromDetails"),
	}),
	}
	db.DetailsInit(ds)
	db.SetText("changed", nil)
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
	t.Run("events", func(t *testing.T) {
		orig := simple.New(testBlock).Copy().(DetailsBlock)
		db := orig.Copy().(DetailsBlock)
		ds := &testDetailsService{details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"title": domain.String("titleFromDetails"),
		}),
		}
		db.DetailsInit(ds)
		db.SetText("changed", nil)
		db.SetChecked(true)
		ok, err := db.ApplyToDetails(orig, ds)
		require.NoError(t, err)
		require.True(t, ok)

		assert.Equal(t, "changed", ds.details.GetString("title"))
		assert.Equal(t, true, ds.details.GetBool("checked"))

		msgs, err := orig.Diff("", db)
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.True(t, msgs[0].Virtual)
		setText := msgs[0].Msg.GetBlockSetText()
		require.NotNil(t, setText)
		require.NotNil(t, setText.Text)
		assert.Equal(t, "changed", setText.Text.Value)
		require.NotNil(t, setText.Checked)
		assert.Equal(t, true, setText.Checked.Value)
	})
	t.Run("change fields only", func(t *testing.T) {
		ds := &testDetailsService{details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"title": domain.String("titleFromDetails"),
		}),
		}
		orig := simple.New(testBlock).Copy().(DetailsBlock)
		orig.DetailsInit(ds)
		db := orig.Copy().(DetailsBlock)

		db.DetailsInit(ds)
		db.Model().Fields = &types.Struct{
			Fields: map[string]*types.Value{
				"keys": pbtypes.String("value"),
			},
		}
		msgs, err := orig.Diff("", db)
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		assert.False(t, msgs[0].Virtual)
	})
}

type testDetailsService struct {
	details *domain.Details
}

func (t *testDetailsService) Details() *domain.Details {
	return t.details
}

func (t *testDetailsService) SetDetail(key domain.RelationKey, value domain.Value) {
	if t.details == nil {
		t.details = domain.NewDetails()
	}
	t.details.Set(key, value)
}
