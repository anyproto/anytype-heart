package filter

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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
		KeyOrder{Key: "a", Type: model.BlockContentDataviewSort_Asc},
		KeyOrder{Key: "b", Type: model.BlockContentDataviewSort_Desc},
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
