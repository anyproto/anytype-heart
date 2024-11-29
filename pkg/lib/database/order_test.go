package database

import (
	"bytes"
	"testing"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func assertCompare(t *testing.T, order Order, a *types.Struct, b *types.Struct, expected int) {
	assert.Equal(t, expected, order.Compare(a, b))
	arena := &anyenc.Arena{}
	aValue := pbtypes.ProtoToAnyEnc(arena, a)
	bValue := pbtypes.ProtoToAnyEnc(arena, b)
	s := order.AnystoreSort()
	aBytes := s.AppendKey(nil, aValue)
	bBytes := s.AppendKey(nil, bValue)
	got := bytes.Compare(aBytes, bBytes)
	assert.Equal(t, expected, got)
}

func TestTextSort(t *testing.T) {
	arena := &anyenc.Arena{}
	t.Run("note layout, not empty name", func(t *testing.T) {
		a := &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyName.String(): pbtypes.String("b"),
			},
		}
		b := &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyName.String():    pbtypes.String("a"),
				bundle.RelationKeySnippet.String(): pbtypes.String("b"),
				bundle.RelationKeyLayout.String():  pbtypes.Int64(int64(model.ObjectType_note)),
			},
		}
		asc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName.String(), Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
		desc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName.String(), Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, desc, a, b, -1)
	})
	t.Run("note layout, empty name", func(t *testing.T) {
		t.Run("one with name, one with snippet, not equal", func(t *testing.T) {
			a := &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyName.String(): pbtypes.String("a"),
				},
			}
			b := &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeySnippet.String(): pbtypes.String("b"),
					bundle.RelationKeyLayout.String():  pbtypes.Int64(int64(model.ObjectType_note)),
				},
			}
			asc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName.String(), Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
			assertCompare(t, asc, a, b, -1)
			desc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName.String(), Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_shorttext}
			assertCompare(t, desc, a, b, 1)
		})
		t.Run("one with name, one with snippet, equal", func(t *testing.T) {
			a := &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeyName.String(): pbtypes.String("a"),
				},
			}
			b := &types.Struct{
				Fields: map[string]*types.Value{
					bundle.RelationKeySnippet.String(): pbtypes.String("a"),
					bundle.RelationKeyLayout.String():  pbtypes.Int64(int64(model.ObjectType_note)),
				},
			}
			asc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName.String(), Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
			assertCompare(t, asc, a, b, 0)
			desc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName.String(), Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_shorttext}
			assertCompare(t, desc, a, b, 0)
		})
	})
}

func TestKeyOrder_Compare(t *testing.T) {
	arena := &anyenc.Arena{}
	t.Run("eq", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 0)
	})
	t.Run("asc", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("b")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_float", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(2)}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_float", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(2)}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_emptylast", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_emptylast_float", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Null()}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_emptylast_str", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_emptylast_str", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_emptyfirst_str", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_emptyfirst_str", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_str_end", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("b")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_str_end", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("b")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_str_start", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("b")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_str_start", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("b")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	date := time.Unix(-1, 0)

	t.Run("asc date, no time", func(t *testing.T) {
		date1 := time.Date(2020, 2, 4, 15, 22, 0, 0, time.UTC)
		date2 := time.Date(2020, 2, 4, 16, 26, 0, 0, time.UTC)

		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date1.Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date2.Unix())}}
		asc := &KeyOrder{arena: arena,
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Asc,
			EmptyPlacement: model.BlockContentDataviewSort_End,
			IncludeTime:    false,
			relationFormat: model.RelationFormat_date,
		}
		assertCompare(t, asc, a, b, 0)
	})

	t.Run("asc_date_end_empty", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena,
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Asc,
			EmptyPlacement: model.BlockContentDataviewSort_End,
			IncludeTime:    false,
			relationFormat: model.RelationFormat_date,
		}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_date_end_empty", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena,
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Desc,
			EmptyPlacement: model.BlockContentDataviewSort_End,
			IncludeTime:    false,
			relationFormat: model.RelationFormat_date,
		}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_date_start_empty", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena,
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Asc,
			EmptyPlacement: model.BlockContentDataviewSort_Start,
			IncludeTime:    false,
			relationFormat: model.RelationFormat_date,
		}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_date_start", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena,
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Desc,
			EmptyPlacement: model.BlockContentDataviewSort_Start,
			IncludeTime:    false,
			relationFormat: model.RelationFormat_date,
		}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_date_start key not exists", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{}}
		asc := &KeyOrder{arena: arena,
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Desc,
			EmptyPlacement: model.BlockContentDataviewSort_Start,
			IncludeTime:    false,
			relationFormat: model.RelationFormat_date,
		}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_date", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(time.Now().Unix())}}
		asc := &KeyOrder{arena: arena,
			Key:            "k",
			Type:           model.BlockContentDataviewSort_Desc,
			EmptyPlacement: model.BlockContentDataviewSort_Start,
			IncludeTime:    false,
			relationFormat: model.RelationFormat_date,
		}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_nil_emptylast", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_nil_emptylast_float", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_nil_emptylast", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_nil_emptylast_float", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_nil_emptylast_float", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_nil_emptyfirst", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_nil_emptyfirst_float", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_nil_emptyfirst", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_nil_emptyfirst_float", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc emptyfirst key not exist", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		b := &types.Struct{Fields: map[string]*types.Value{}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_nil", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": nil}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(0)}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_notspecified", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_notspecified", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(0)}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_notspecified", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("b")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_notspecified_float", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(0)}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Float64(1)}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})
}

func TestKeyUnicodeOrder_Compare(t *testing.T) {
	arena := &anyenc.Arena{}
	t.Run("asc", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("Єгипет")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("Японія")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("dsc", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("Ürkmez")}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("Zurich")}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})
}

func TestSetOrder_Compare(t *testing.T) {
	arena := &anyenc.Arena{}
	so := SetOrder{
		&KeyOrder{arena: arena, Key: "a", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext},
		&KeyOrder{arena: arena, Key: "b", Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_shorttext},
	}
	t.Run("eq", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"a": pbtypes.String("a"), "b": pbtypes.String("b")}}
		b := &types.Struct{Fields: map[string]*types.Value{"a": pbtypes.String("a"), "b": pbtypes.String("b")}}
		assertCompare(t, so, a, b, 0)
	})
	t.Run("first order", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"a": pbtypes.String("b"), "b": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"a": pbtypes.String("a"), "b": pbtypes.String("b")}}
		assertCompare(t, so, a, b, 1)

	})
	t.Run("second order", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"a": pbtypes.String("b"), "b": pbtypes.String("b")}}
		b := &types.Struct{Fields: map[string]*types.Value{"a": pbtypes.String("b"), "b": pbtypes.String("a")}}
		assertCompare(t, so, a, b, -1)
	})
}

func TestCustomOrder_Compare(t *testing.T) {
	a := &anyenc.Arena{}
	// keys are json values
	idxIndices := map[string]int{
		string(a.NewString("b").MarshalTo(nil)): 0,
		string(a.NewString("c").MarshalTo(nil)): 1,
		string(a.NewString("d").MarshalTo(nil)): 2,
		string(a.NewString("a").MarshalTo(nil)): 3,
	}
	arena := &anyenc.Arena{}
	co := newCustomOrder(arena, "ID", idxIndices, &KeyOrder{arena: arena, Key: "ID", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext})

	t.Run("gt", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("c")}}
		b := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("a")}}
		assertCompare(t, co, a, b, -1)
	})

	t.Run("eq", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("a")}}
		assertCompare(t, co, a, b, 0)
	})

	t.Run("lt", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("b")}}
		assertCompare(t, co, a, b, 1)
	})

	t.Run("first found second not", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("a")}}
		b := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("x")}}
		assertCompare(t, co, a, b, -1)
	})

	t.Run("first not found second yes", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("x")}}
		b := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("a")}}
		assertCompare(t, co, a, b, 1)
	})

	t.Run("both not found gt", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("y")}}
		b := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("z")}}
		assertCompare(t, co, a, b, -1)
	})

	t.Run("both not found eq", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("z")}}
		b := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("z")}}
		assertCompare(t, co, a, b, 0)
	})

	t.Run("both not found lt", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("z")}}
		b := &types.Struct{Fields: map[string]*types.Value{"ID": pbtypes.String("y")}}
		assertCompare(t, co, a, b, 1)
	})
}

func TestTagStatusOrder_Compare(t *testing.T) {
	arena := &anyenc.Arena{}
	for _, relation := range []model.RelationFormat{model.RelationFormat_tag, model.RelationFormat_status} {
		t.Run("eq", func(t *testing.T) {
			a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
			b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
			asc := &KeyOrder{arena: arena,
				Key:            "k",
				Type:           model.BlockContentDataviewSort_Asc,
				relationFormat: relation,
				Options:        map[string]string{"a": "a"},
			}
			assertCompare(t, asc, a, b, 0)
		})

		t.Run("asc", func(t *testing.T) {
			a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("b")}}
			b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.String("a")}}
			asc := &KeyOrder{arena: arena,
				Key:            "k",
				Type:           model.BlockContentDataviewSort_Asc,
				relationFormat: relation,
				Options: map[string]string{
					"b": "a",
					"a": "b",
				},
			}
			assertCompare(t, asc, a, b, -1)
		})
	}
}

func TestIncludeTime_Compare(t *testing.T) {
	date := time.Unix(1672012800, 0)
	arena := &anyenc.Arena{}
	t.Run("date only eq", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Add(time.Second * 5).Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Add(time.Second * 10).Unix())}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: false, relationFormat: model.RelationFormat_date}
		assertCompare(t, asc, a, b, 0)
	})

	t.Run("only date lt", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Add(time.Hour * 24).Unix())}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: false, relationFormat: model.RelationFormat_date}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("date includeTime eq", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Add(time.Second * 10).Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Add(time.Second * 10).Unix())}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: true, relationFormat: model.RelationFormat_date}
		assertCompare(t, asc, a, b, 0)
	})

	t.Run("date includeTime lt", func(t *testing.T) {
		a := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Add(time.Second * 5).Unix())}}
		b := &types.Struct{Fields: map[string]*types.Value{"k": pbtypes.Int64(date.Add(time.Second * 10).Unix())}}
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: true, relationFormat: model.RelationFormat_date}
		assertCompare(t, asc, a, b, -1)
	})

}
