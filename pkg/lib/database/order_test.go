package database

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestKeyOrder_Compare(t *testing.T) {
	t.Run("eq", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("a")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc}
		assert.Equal(t, 0, asc.Compare(a, b))
	})
	t.Run("asc", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("b")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("desc_float", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Float64(1)}
		b := testGetter{"k": pbtypes.Float64(2)}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("asc_float", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Float64(1)}
		b := testGetter{"k": pbtypes.Float64(2)}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_emptylast", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_emptylast_float", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Float64(1)}
		b := testGetter{"k": pbtypes.Null()}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_emptylast_str", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("desc_emptylast_str", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_emptyfirst_str", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("desc_emptyfirst_str", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("asc_str_end", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("b")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("desc_str_end", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("b")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("asc_str_start", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("b")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("desc_str_start", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("b")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	date := time.Unix(-1, 0)

	t.Run("asc_date_end_empty", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Int64(date.Unix())}
		b := testGetter{"k": nil}
		asc := KeyOrder{
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Asc,
			EmptyPlacement: model.BlockContentDataviewSort_End,
			IncludeTime:    false,
			RelationFormat: model.RelationFormat_date,
		}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("desc_date_end_empty", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Int64(date.Unix())}
		b := testGetter{"k": nil}
		asc := KeyOrder{
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Desc,
			EmptyPlacement: model.BlockContentDataviewSort_End,
			IncludeTime:    false,
			RelationFormat: model.RelationFormat_date,
		}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_date_start_empty", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Int64(date.Unix())}
		b := testGetter{"k": nil}
		asc := KeyOrder{
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Asc,
			EmptyPlacement: model.BlockContentDataviewSort_Start,
			IncludeTime:    false,
			RelationFormat: model.RelationFormat_date,
		}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("desc_date_start", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Int64(date.Unix())}
		b := testGetter{"k": nil}
		asc := KeyOrder{
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Desc,
			EmptyPlacement: model.BlockContentDataviewSort_Start,
			IncludeTime:    false,
			RelationFormat: model.RelationFormat_date,
		}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("asc_nil_emptylast", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": nil}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_nil_emptylast_float", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Float64(1)}
		b := testGetter{"k": nil}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("desc_nil_emptylast", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": nil}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("desc_nil_emptylast_float", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Float64(1)}
		b := testGetter{"k": nil}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_nil_emptyfirst", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": nil}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("asc_nil_emptyfirst_float", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Float64(1)}
		b := testGetter{"k": nil}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("desc_nil_emptyfirst", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": nil}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("desc_nil_emptyfirst_float", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Float64(1)}
		b := testGetter{"k": nil}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("asc_nil", func(t *testing.T) {
		a := testGetter{"k": nil}
		b := testGetter{"k": pbtypes.String("")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_nil", func(t *testing.T) {
		a := testGetter{"k": nil}
		b := testGetter{"k": pbtypes.Float64(0)}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_notspecified", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("asc_notspecified", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Float64(1)}
		b := testGetter{"k": pbtypes.Float64(0)}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("desc_notspecified", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("b")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("desc_notspecified_float", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Float64(0)}
		b := testGetter{"k": pbtypes.Float64(1)}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc}
		assert.Equal(t, 1, asc.Compare(a, b))
	})
}

func TestKeyUnicodeOrder_Compare(t *testing.T) {
	t.Run("asc", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("Єгипет")}
		b := testGetter{"k": pbtypes.String("Японія")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("dsc", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("Ürkmez")}
		b := testGetter{"k": pbtypes.String("Zurich")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Desc}
		assert.Equal(t, 1, asc.Compare(a, b))
	})
}

func TestSetOrder_Compare(t *testing.T) {
	so := SetOrder{
		&KeyOrder{Key: "a", Type: model.BlockContentDataviewSort_Asc},
		&KeyOrder{Key: "b", Type: model.BlockContentDataviewSort_Desc},
	}
	t.Run("eq", func(t *testing.T) {
		a := testGetter{"a": pbtypes.String("a"), "b": pbtypes.String("b")}
		b := testGetter{"a": pbtypes.String("a"), "b": pbtypes.String("b")}
		assert.Equal(t, 0, so.Compare(a, b))
	})
	t.Run("first order", func(t *testing.T) {
		a := testGetter{"a": pbtypes.String("b"), "b": pbtypes.String("a")}
		b := testGetter{"a": pbtypes.String("a"), "b": pbtypes.String("b")}
		assert.Equal(t, 1, so.Compare(a, b))
	})
	t.Run("second order", func(t *testing.T) {
		a := testGetter{"a": pbtypes.String("b"), "b": pbtypes.String("b")}
		b := testGetter{"a": pbtypes.String("b"), "b": pbtypes.String("a")}
		assert.Equal(t, -1, so.Compare(a, b))
	})
}

func TestCustomOrder_Compare(t *testing.T) {
	needOrder := []*types.Value{
		pbtypes.String("b"),
		pbtypes.String("c"),
		pbtypes.String("d"),
		pbtypes.String("a"),
	}
	co := NewCustomOrder("ID", needOrder, KeyOrder{Key: "ID", Type: model.BlockContentDataviewSort_Asc})

	t.Run("gt", func(t *testing.T) {
		a := testGetter{"ID": pbtypes.String("c")}
		b := testGetter{"ID": pbtypes.String("a")}
		assert.Equal(t, -1, co.Compare(a, b))
	})

	t.Run("eq", func(t *testing.T) {
		a := testGetter{"ID": pbtypes.String("a")}
		b := testGetter{"ID": pbtypes.String("a")}
		assert.Equal(t, 0, co.Compare(a, b))
	})

	t.Run("lt", func(t *testing.T) {
		a := testGetter{"ID": pbtypes.String("a")}
		b := testGetter{"ID": pbtypes.String("b")}
		assert.Equal(t, 1, co.Compare(a, b))
	})

	t.Run("first found second not", func(t *testing.T) {
		a := testGetter{"ID": pbtypes.String("a")}
		b := testGetter{"ID": pbtypes.String("x")}
		assert.Equal(t, -1, co.Compare(a, b))
	})

	t.Run("first not found second yes", func(t *testing.T) {
		a := testGetter{"ID": pbtypes.String("x")}
		b := testGetter{"ID": pbtypes.String("a")}
		assert.Equal(t, 1, co.Compare(a, b))
	})

	t.Run("both not found gt", func(t *testing.T) {
		a := testGetter{"ID": pbtypes.String("y")}
		b := testGetter{"ID": pbtypes.String("z")}
		assert.Equal(t, -1, co.Compare(a, b))
	})

	t.Run("both not found eq", func(t *testing.T) {
		a := testGetter{"ID": pbtypes.String("z")}
		b := testGetter{"ID": pbtypes.String("z")}
		assert.Equal(t, 0, co.Compare(a, b))
	})

	t.Run("both not found lt", func(t *testing.T) {
		a := testGetter{"ID": pbtypes.String("z")}
		b := testGetter{"ID": pbtypes.String("y")}
		assert.Equal(t, 1, co.Compare(a, b))
	})
}

func TestTagStatusOrder_Compare(t *testing.T) {

	for _, relation := range []model.RelationFormat{model.RelationFormat_tag, model.RelationFormat_status} {
		t.Run("eq", func(t *testing.T) {
			a := testGetter{"k": pbtypes.String("a")}
			b := testGetter{"k": pbtypes.String("a")}
			asc := KeyOrder{
				Key:            "k",
				Type:           model.BlockContentDataviewSort_Asc,
				RelationFormat: relation,
				Options:        map[string]string{"a": "a"},
			}
			assert.Equal(t, 0, asc.Compare(a, b))
		})

		t.Run("asc", func(t *testing.T) {
			a := testGetter{"k": pbtypes.String("b")}
			b := testGetter{"k": pbtypes.String("a")}
			asc := KeyOrder{
				Key:            "k",
				Type:           model.BlockContentDataviewSort_Asc,
				RelationFormat: relation,
				Options: map[string]string{
					"b": "a",
					"a": "b",
				},
			}
			assert.Equal(t, -1, asc.Compare(a, b))
		})
	}
}

func TestIncludeTime_Compare(t *testing.T) {
	date := time.Unix(1672012800, 0)

	t.Run("date only eq", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Int64(date.Add(time.Second * 5).Unix())}
		b := testGetter{"k": pbtypes.Int64(date.Add(time.Second * 10).Unix())}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: false, RelationFormat: model.RelationFormat_date}
		assert.Equal(t, 0, asc.Compare(a, b))
	})

	t.Run("only date lt", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Int64(date.Unix())}
		b := testGetter{"k": pbtypes.Int64(date.Add(time.Hour * 24).Unix())}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: false, RelationFormat: model.RelationFormat_date}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("date includeTime eq", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Int64(date.Add(time.Second * 10).Unix())}
		b := testGetter{"k": pbtypes.Int64(date.Add(time.Second * 10).Unix())}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: true, RelationFormat: model.RelationFormat_date}
		assert.Equal(t, 0, asc.Compare(a, b))
	})

	t.Run("date includeTime lt", func(t *testing.T) {
		a := testGetter{"k": pbtypes.Int64(date.Add(time.Second * 5).Unix())}
		b := testGetter{"k": pbtypes.Int64(date.Add(time.Second * 10).Unix())}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: true, RelationFormat: model.RelationFormat_date}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

}
