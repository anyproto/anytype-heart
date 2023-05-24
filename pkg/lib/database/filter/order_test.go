package filter

import (
	"github.com/gogo/protobuf/types"
	"testing"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/stretchr/testify/assert"
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

	t.Run("asc_emptylast", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyLast: true}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_nil_emptylast", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": nil}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyLast: true}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_nil", func(t *testing.T) {
		a := testGetter{"k": nil}
		b := testGetter{"k": pbtypes.String("")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc}
		assert.Equal(t, -1, asc.Compare(a, b))
	})

	t.Run("asc_notemptylast", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("")}
		asc := KeyOrder{Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyLast: false}
		assert.Equal(t, 1, asc.Compare(a, b))
	})

	t.Run("desc", func(t *testing.T) {
		a := testGetter{"k": pbtypes.String("a")}
		b := testGetter{"k": pbtypes.String("b")}
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
