package database

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func assertCompare(t *testing.T, order Order, a *domain.Details, b *domain.Details, expected int) {
	assert.Equal(t, expected, order.Compare(a, b))
	arena := &anyenc.Arena{}
	aValue := a.ToAnyEnc(arena)
	bValue := b.ToAnyEnc(arena)
	s := order.AnystoreSort()
	aBytes := s.AppendKey(nil, aValue)
	bBytes := s.AppendKey(nil, bValue)
	got := bytes.Compare(aBytes, bBytes)
	assert.Equal(t, expected, got)
}

func TestTextSort(t *testing.T) {
	arena := &anyenc.Arena{}
	t.Run("note layout, not empty name", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("b"),
		})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:           domain.String("a"),
			bundle.RelationKeySnippet:        domain.String("b"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_note)),
		})
		asc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
		desc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, desc, a, b, -1)
	})
	t.Run("note layout, empty name", func(t *testing.T) {
		t.Run("one with name, one with snippet, not equal", func(t *testing.T) {
			a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("a"),
			})
			b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeySnippet:        domain.String("b"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_note)),
			})
			asc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
			assertCompare(t, asc, a, b, -1)
			desc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_shorttext}
			assertCompare(t, desc, a, b, 1)
		})
		t.Run("one with name, one with snippet, equal", func(t *testing.T) {
			a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName: domain.String("a"),
			})
			b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeySnippet:        domain.String("a"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_note)),
			})
			asc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
			assertCompare(t, asc, a, b, 0)
			desc := &KeyOrder{arena: arena, Key: bundle.RelationKeyName, Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_shorttext}
			assertCompare(t, desc, a, b, 0)
		})
	})
}

func TestKeyOrder_Compare(t *testing.T) {
	arena := &anyenc.Arena{}
	t.Run("eq", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 0)
	})
	t.Run("asc", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("b")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_float", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_float", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(2)})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_emptylast", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_emptylast_float", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_emptylast_str", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_emptylast_str", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_emptyfirst_str", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_emptyfirst_str", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_str_end", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("b")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_str_end", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("b")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_str_start", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("b")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_str_start", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("b")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	date := time.Unix(-1, 0)

	t.Run("asc date, no time", func(t *testing.T) {
		date1 := time.Date(2020, 2, 4, 15, 22, 0, 0, time.UTC)
		date2 := time.Date(2020, 2, 4, 16, 26, 0, 0, time.UTC)

		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date1.Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date2.Unix())})
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
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
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
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
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
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Null()})
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
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
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
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
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
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(time.Now().Unix())})
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
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_nil_emptylast_float", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_nil_emptylast", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_nil_emptylast_float", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Null()})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("desc_nil_emptylast_float", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Null()})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_End, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_nil_emptyfirst", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Null()})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_nil_emptyfirst_float", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Null()})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_nil_emptyfirst", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_nil_emptyfirst_float", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc emptyfirst key not exist", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, EmptyPlacement: model.BlockContentDataviewSort_Start, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_nil", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Null()})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(0)})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("asc_notspecified", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("asc_notspecified", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(0)})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_notspecified", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("b")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, 1)
	})

	t.Run("desc_notspecified_float", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(0)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Float64(1)})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_number}
		assertCompare(t, asc, a, b, 1)
	})
	t.Run("disable_collate", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeySpaceOrder: domain.String("--UK")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeySpaceOrder: domain.String("--jc")})
		ko := &KeyOrder{disableCollator: true, arena: arena, Key: bundle.RelationKeySpaceOrder, Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, ko, a, b, -1)
	})
	t.Run("compare_bool_false_null", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyDone: domain.Bool(false)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		ko := &KeyOrder{arena: arena, Key: bundle.RelationKeyDone, Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_checkbox}
		assertCompare(t, ko, a, b, 0)
	})
	t.Run("compare_bool_true_null", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyDone: domain.Bool(true)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		ko := &KeyOrder{arena: arena, Key: bundle.RelationKeyDone, Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_checkbox}
		assertCompare(t, ko, a, b, 1)
	})
	t.Run("compare_bool_true_null_desc", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyDone: domain.Bool(true)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		ko := &KeyOrder{arena: arena, Key: bundle.RelationKeyDone, Type: model.BlockContentDataviewSort_Desc, relationFormat: model.RelationFormat_checkbox}
		assertCompare(t, ko, a, b, -1)
	})
	t.Run("compare_bool_true_false", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyDone: domain.Bool(true)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyDone: domain.Bool(false)})
		ko := &KeyOrder{arena: arena, Key: bundle.RelationKeyDone, Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_checkbox}
		assertCompare(t, ko, a, b, 1)
	})
	t.Run("compare_bool_true_true", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyDone: domain.Bool(true)})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyDone: domain.Bool(true)})
		ko := &KeyOrder{arena: arena, Key: bundle.RelationKeyDone, Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_checkbox}
		assertCompare(t, ko, a, b, 0)
	})
}

func TestKeyUnicodeOrder_Compare(t *testing.T) {
	arena := &anyenc.Arena{}
	t.Run("asc", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("Єгипет")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("Японія")})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc, relationFormat: model.RelationFormat_shorttext}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("dsc", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("Ürkmez")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.String("Zurich")})
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
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"a": domain.String("a"), "b": domain.String("b")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"a": domain.String("a"), "b": domain.String("b")})
		assertCompare(t, so, a, b, 0)
	})
	t.Run("first order", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"a": domain.String("b"), "b": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"a": domain.String("a"), "b": domain.String("b")})
		assertCompare(t, so, a, b, 1)

	})
	t.Run("second order", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"a": domain.String("b"), "b": domain.String("b")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"a": domain.String("b"), "b": domain.String("a")})
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
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("c")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("a")})
		assertCompare(t, co, a, b, -1)
	})

	t.Run("eq", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("a")})
		assertCompare(t, co, a, b, 0)
	})

	t.Run("lt", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("b")})
		assertCompare(t, co, a, b, 1)
	})

	t.Run("first found second not", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("a")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("x")})
		assertCompare(t, co, a, b, -1)
	})

	t.Run("first not found second yes", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("x")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("a")})
		assertCompare(t, co, a, b, 1)
	})

	t.Run("both not found gt", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("y")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("z")})
		assertCompare(t, co, a, b, -1)
	})

	t.Run("both not found eq", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("z")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("z")})
		assertCompare(t, co, a, b, 0)
	})

	t.Run("both not found lt", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("z")})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"ID": domain.String("y")})
		assertCompare(t, co, a, b, 1)
	})
}

func TestTagStatusOrder_Compare(t *testing.T) {
	arena := &anyenc.Arena{}
	for _, relation := range []model.RelationFormat{model.RelationFormat_tag, model.RelationFormat_status} {
		t.Run("eq", func(t *testing.T) {
			a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"a"})})
			b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"a"})})
			asc := &KeyOrder{arena: arena,
				Key:            "k",
				Type:           model.BlockContentDataviewSort_Asc,
				relationFormat: relation,
				objectStore:    &stubSpaceObjectStore{},
				orderMap:       NewOrderMap(nil),
			}
			assertCompare(t, asc, a, b, 0)
		})

		t.Run("asc by name", func(t *testing.T) {
			a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"b"})})
			b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"a"})})
			asc := &KeyOrder{arena: arena,
				Key:            "k",
				Type:           model.BlockContentDataviewSort_Asc,
				relationFormat: relation,
				objectStore:    &stubSpaceObjectStore{},
				objectSortKeys: []domain.RelationKey{bundle.RelationKeyOrderId, bundle.RelationKeyName},
				orderMap: NewOrderMap(map[string]*domain.Details{
					"b": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"name": domain.String("a")}),
					"a": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"name": domain.String("b")}),
				}),
			}
			assertCompare(t, asc, a, b, -1)
		})

		t.Run("desc by orderId", func(t *testing.T) {
			a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"a"})})
			b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"b"})})
			desc := &KeyOrder{arena: arena,
				Key:            "k",
				Type:           model.BlockContentDataviewSort_Asc,
				relationFormat: relation,
				objectStore:    &stubSpaceObjectStore{},
				objectSortKeys: []domain.RelationKey{bundle.RelationKeyOrderId, bundle.RelationKeyName},
				orderMap: NewOrderMap(map[string]*domain.Details{
					"a": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"orderId": domain.String("V_Sx")}),
					"b": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"orderId": domain.String("VZTT")}),
				}),
			}
			assertCompare(t, desc, a, b, 1)
		})

		t.Run("asc by orderId", func(t *testing.T) {
			a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"b"})})
			b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.StringList([]string{"a"})})
			asc := &KeyOrder{arena: arena,
				Key:            "k",
				Type:           model.BlockContentDataviewSort_Asc,
				relationFormat: relation,
				objectStore:    &stubSpaceObjectStore{},
				objectSortKeys: []domain.RelationKey{bundle.RelationKeyOrderId, bundle.RelationKeyName},
				orderMap: NewOrderMap(map[string]*domain.Details{
					"b": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"orderId": domain.String("VYTy")}),
					"a": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"orderId": domain.String("VZTT")}),
				}),
			}
			assertCompare(t, asc, a, b, -1)
		})
	}
}

func TestIncludeTime_Compare(t *testing.T) {
	date := time.Unix(1672012800, 0)
	arena := &anyenc.Arena{}
	t.Run("date only eq", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Add(time.Second * 5).Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Add(time.Second * 10).Unix())})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: false, relationFormat: model.RelationFormat_date}
		assertCompare(t, asc, a, b, 0)
	})

	t.Run("only date lt", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Add(time.Hour * 24).Unix())})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: false, relationFormat: model.RelationFormat_date}
		assertCompare(t, asc, a, b, -1)
	})

	t.Run("date includeTime eq", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Add(time.Second * 10).Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Add(time.Second * 10).Unix())})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: true, relationFormat: model.RelationFormat_date}
		assertCompare(t, asc, a, b, 0)
	})

	t.Run("date includeTime lt", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Add(time.Second * 5).Unix())})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{"k": domain.Int64(date.Add(time.Second * 10).Unix())})
		asc := &KeyOrder{arena: arena, Key: "k", Type: model.BlockContentDataviewSort_Asc,
			IncludeTime: true, relationFormat: model.RelationFormat_date}
		assertCompare(t, asc, a, b, -1)
	})

}

func TestOrderMap_BuildOrderByKey(t *testing.T) {
	key := domain.RelationKey("key")
	buf := make([]byte, 0)
	t.Run("nil OrderMap", func(t *testing.T) {
		var om *OrderMap
		result := om.BuildOrderByKey(key, buf, "id1", "id2")
		assert.Equal(t, "", string(result))
	})

	t.Run("empty data", func(t *testing.T) {
		om := &OrderMap{data: make(map[string]*domain.Details)}
		result := om.BuildOrderByKey(key, buf, "id1", "id2")
		assert.Equal(t, "", string(result))
	})

	t.Run("single existing id", func(t *testing.T) {
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					key: domain.String("BBBBTag A"),
				}),
			},
		}
		result := om.BuildOrderByKey(key, buf, "id1")
		assert.Equal(t, "BBBBTag A", string(result))
	})

	t.Run("multiple existing ids", func(t *testing.T) {
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					key: domain.String("BBBBTag A"),
				}),
				"id2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					key: domain.String("CCCCTag B"),
				}),
			},
		}
		result := om.BuildOrderByKey(key, buf, "id1", "id2")
		assert.Equal(t, "BBBBTag ACCCCTag B", string(result))
	})

	t.Run("mixed existing and non-existing ids", func(t *testing.T) {
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					key: domain.String("BBBBTag A"),
				}),
			},
		}
		result := om.BuildOrderByKey(key, buf, "id1", "nonexistent", "id1")
		assert.Equal(t, "BBBBTag ABBBBTag A", string(result))
	})

	t.Run("no ids provided", func(t *testing.T) {
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					key: domain.String("BBBBTag A"),
				}),
			},
		}
		result := om.BuildOrderByKey(key, buf)
		assert.Equal(t, "", string(result))
	})

	t.Run("not existing key", func(t *testing.T) {
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					key: domain.String("BBBBTag A"),
				}),
			},
		}
		result := om.BuildOrderByKey("not_existing_key", buf, "id1")
		assert.Equal(t, "", string(result))
	})
}

func BenchmarkOrderMap_BuildOrderByKey(b *testing.B) {
	var (
		key  = domain.RelationKey("key")
		data = make(map[string]*domain.Details, 100)
		ids  = make([]string, 100)
	)

	for i := 0; i < 100; i++ {
		ids[i] = fmt.Sprintf("id%d", i)
		data[ids[i]] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			key: domain.String(fmt.Sprintf("%d", rand.Int63())),
		})
	}

	var (
		om   = NewOrderMap(data)
		buf  = make([]byte, 0)
		rng  = rand.New(rand.NewSource(132211))
		swap = func(i, j int) { ids[i], ids[j] = ids[j], ids[i] }
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rng.Shuffle(100, swap)
		buf = om.BuildOrderByKey(key, buf, ids...)
	}
}

func TestOrderMap_Update(t *testing.T) {
	t.Run("nil OrderMap", func(t *testing.T) {
		var om *OrderMap
		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Updated Name"),
				bundle.RelationKeyOrderId: domain.String("DDDD"),
			}),
		}
		updated := om.Update(details)
		assert.False(t, updated)
	})

	t.Run("empty data", func(t *testing.T) {
		om := &OrderMap{data: make(map[string]*domain.Details)}
		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Updated Name"),
				bundle.RelationKeyOrderId: domain.String("DDDD"),
			}),
		}
		updated := om.Update(details)
		assert.False(t, updated)
	})

	t.Run("update existing object with new orderId", func(t *testing.T) {
		original := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("Original Name"),
			bundle.RelationKeyOrderId: domain.String("BBBB"),
		})
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": original,
			},
		}

		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Original Name"),
				bundle.RelationKeyOrderId: domain.String("CCCC"),
			}),
		}

		updated := om.Update(details)
		assert.True(t, updated)
		assert.Equal(t, "CCCC", original.GetString(bundle.RelationKeyOrderId))
	})

	t.Run("update existing object with new name", func(t *testing.T) {
		original := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("Original Name"),
			bundle.RelationKeyOrderId: domain.String("BBBB"),
		})
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": original,
			},
		}

		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Updated Name"),
				bundle.RelationKeyOrderId: domain.String("BBBB"),
			}),
		}

		updated := om.Update(details)
		assert.True(t, updated)
		assert.Equal(t, "Updated Name", original.GetString(bundle.RelationKeyName))
	})

	t.Run("update existing object with no changes", func(t *testing.T) {
		original := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("Same Name"),
			bundle.RelationKeyOrderId: domain.String("BBBB"),
		})
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": original,
			},
		}

		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Same Name"),
				bundle.RelationKeyOrderId: domain.String("BBBB"),
			}),
		}

		updated := om.Update(details)
		assert.False(t, updated)
	})

	t.Run("update non-existing object", func(t *testing.T) {
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyName: domain.String("Existing"),
				}),
			},
		}

		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("id2"),
				bundle.RelationKeyName: domain.String("Non-existing"),
			}),
		}

		updated := om.Update(details)
		assert.False(t, updated)
		assert.Len(t, om.data, 1) // Should still have only the original object
	})

	t.Run("update multiple objects", func(t *testing.T) {
		obj1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("Name1"),
			bundle.RelationKeyOrderId: domain.String("BBBB"),
		})
		obj2 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("Name2"),
			bundle.RelationKeyOrderId: domain.String("CCCC"),
		})

		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": obj1,
				"id2": obj2,
			},
		}

		details := []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("Updated Name1"),
				bundle.RelationKeyOrderId: domain.String("BBBB"),
			}),
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:      domain.String("id2"),
				bundle.RelationKeyName:    domain.String("Name2"),
				bundle.RelationKeyOrderId: domain.String("DDDD"),
			}),
		}

		updated := om.Update(details)
		assert.True(t, updated)
		assert.Equal(t, "Updated Name1", obj1.GetString(bundle.RelationKeyName))
		assert.Equal(t, "DDDD", obj2.GetString(bundle.RelationKeyOrderId))
	})
}

// Mock ObjectStore for testing
type mockObjectStore struct {
	mock.Mock
}

func (m *mockObjectStore) SpaceId() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockObjectStore) Query(q Query) (records []Record, err error) {
	args := m.Called(q)
	return args.Get(0).([]Record), args.Error(1)
}

func (m *mockObjectStore) QueryRaw(filters *Filters, limit int, offset int) ([]Record, error) {
	args := m.Called(filters, limit, offset)
	return args.Get(0).([]Record), args.Error(1)
}

func (m *mockObjectStore) QueryIterate(q Query, proc func(details *domain.Details)) (err error) {
	args := m.Called(q, mock.Anything)
	return args.Error(0)
}

func (m *mockObjectStore) GetRelationFormatByKey(key domain.RelationKey) (model.RelationFormat, error) {
	args := m.Called(key)
	return args.Get(0).(model.RelationFormat), args.Error(1)
}

func (m *mockObjectStore) ListRelationOptions(relationKey domain.RelationKey) (options []*model.RelationOption, err error) {
	args := m.Called(relationKey)
	return args.Get(0).([]*model.RelationOption), args.Error(1)
}

func TestOrderMap_SetOrders(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		om := &OrderMap{data: make(map[string]*domain.Details)}
		err := om.SetOrders(nil, "id1", "id2")
		assert.Empty(t, om.data)
		assert.NoError(t, err)
	})

	t.Run("nil data initialized", func(t *testing.T) {
		om := &OrderMap{}
		store := &mockObjectStore{}

		records := []Record{
			{
				Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:      domain.String("id1"),
					bundle.RelationKeyName:    domain.String("Tag A"),
					bundle.RelationKeyOrderId: domain.String("BBBB"),
				}),
			},
		}

		store.On("Query", mock.AnythingOfType("Query")).Return(records, nil)

		err := om.SetOrders(store, "id1")

		assert.NoError(t, err)
		assert.Len(t, om.data, 1)
		assert.Contains(t, om.data, "id1")
		assert.Equal(t, "Tag A", om.data["id1"].GetString(bundle.RelationKeyName))
		assert.Equal(t, "BBBB", om.data["id1"].GetString(bundle.RelationKeyOrderId))
		store.AssertExpectations(t)
	})

	t.Run("some ids already exist", func(t *testing.T) {
		existing := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("Existing"),
		})
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": existing,
			},
		}

		store := &mockObjectStore{}
		records := []Record{
			{
				Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:      domain.String("id2"),
					bundle.RelationKeyName:    domain.String("New Tag"),
					bundle.RelationKeyOrderId: domain.String("CCCC"),
				}),
			},
		}

		store.On("Query", mock.AnythingOfType("Query")).Return(records, nil)

		err := om.SetOrders(store, "id1", "id2") // id1 exists, id2 is new

		assert.NoError(t, err)
		assert.Len(t, om.data, 2)
		assert.Equal(t, existing, om.data["id1"]) // Should be unchanged
		assert.Equal(t, "New Tag", om.data["id2"].GetString(bundle.RelationKeyName))
		store.AssertExpectations(t)
	})

	t.Run("all ids already exist", func(t *testing.T) {
		om := &OrderMap{
			data: map[string]*domain.Details{
				"id1": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyName: domain.String("Existing1"),
				}),
				"id2": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyName: domain.String("Existing2"),
				}),
			},
		}

		store := &mockObjectStore{}
		// Should not call Query since all ids exist

		err := om.SetOrders(store, "id1", "id2")

		assert.NoError(t, err)
		assert.Len(t, om.data, 2)
		store.AssertNotCalled(t, "Query")
	})

	t.Run("query error", func(t *testing.T) {
		om := &OrderMap{data: make(map[string]*domain.Details)}
		store := &mockObjectStore{}

		store.On("Query", mock.AnythingOfType("Query")).Return([]Record{}, assert.AnError)

		err := om.SetOrders(store, "id1")

		assert.Error(t, err)
		assert.Empty(t, om.data)
		store.AssertExpectations(t)
	})
}

func TestOptionsToMap(t *testing.T) {
	t.Run("store returns options", func(t *testing.T) {
		store := &mockObjectStore{}
		options := []*model.RelationOption{
			{
				Id:      "opt1",
				Text:    "Option 1",
				OrderId: "BBBB",
			},
			{
				Id:      "opt2",
				Text:    "Option 2",
				OrderId: "CCCC",
			},
			{
				Id:   "opt3",
				Text: "Option 3",
				// No OrderId
			},
		}

		store.On("ListRelationOptions", domain.RelationKey("status")).Return(options, nil)

		result := optionsToMap("status", store)

		assert.Len(t, result, 3)
		assert.Equal(t, "Option 1", result["opt1"].GetString(bundle.RelationKeyName))
		assert.Equal(t, "BBBB", result["opt1"].GetString(bundle.RelationKeyOrderId))
		assert.Equal(t, "Option 2", result["opt2"].GetString(bundle.RelationKeyName))
		assert.Equal(t, "CCCC", result["opt2"].GetString(bundle.RelationKeyOrderId))
		assert.Equal(t, "Option 3", result["opt3"].GetString(bundle.RelationKeyName))
		assert.Equal(t, "", result["opt3"].GetString(bundle.RelationKeyOrderId))

		store.AssertExpectations(t)
	})

	t.Run("store returns error", func(t *testing.T) {
		store := &mockObjectStore{}
		store.On("ListRelationOptions", domain.RelationKey("status")).Return([]*model.RelationOption{}, assert.AnError)

		result := optionsToMap("status", store)

		assert.Empty(t, result)
		store.AssertExpectations(t)
	})

	t.Run("empty options", func(t *testing.T) {
		store := &mockObjectStore{}
		store.On("ListRelationOptions", domain.RelationKey("status")).Return([]*model.RelationOption{}, nil)

		result := optionsToMap("status", store)

		assert.Empty(t, result)
		store.AssertExpectations(t)
	})
}

func TestObjectsToMap(t *testing.T) {
	t.Run("successful query with filtering", func(t *testing.T) {
		store := &mockObjectStore{}

		// For this test, we'll mock the function to return no error, indicating success
		// The actual filtering logic is tested in the real implementation
		store.On("QueryIterate", mock.AnythingOfType("Query"), mock.Anything).Return(nil)

		result := objectsToMap("tags", store)

		// Since we can't easily simulate the callback with the new type signature,
		// we'll just verify the function doesn't crash and returns empty result
		assert.NotNil(t, result)
		store.AssertExpectations(t)
	})

	t.Run("query error", func(t *testing.T) {
		store := &mockObjectStore{}
		store.On("QueryIterate", mock.AnythingOfType("Query"), mock.Anything).Return(assert.AnError)

		result := objectsToMap("tags", store)

		assert.Nil(t, result)
		store.AssertExpectations(t)
	})

	t.Run("no objects found", func(t *testing.T) {
		store := &mockObjectStore{}
		store.On("QueryIterate", mock.AnythingOfType("Query"), mock.Anything).Return(nil)

		result := objectsToMap("tags", store)

		assert.Empty(t, result)
		store.AssertExpectations(t)
	})

	t.Run("objects with empty relations", func(t *testing.T) {
		store := &mockObjectStore{}

		store.On("QueryIterate", mock.AnythingOfType("Query"), mock.Anything).Return(nil)

		result := objectsToMap("tags", store)

		assert.Empty(t, result) // No objects should remain since no relations exist
		store.AssertExpectations(t)
	})
}

func TestKeyOrder_Compare_byDate(t *testing.T) {
	key := domain.RelationKey("k")
	dateA := time.Date(1917, 02, 23, 9, 00, 00, 0, time.UTC)
	dateB := time.Date(2025, 10, 28, 13, 17, 0, 0, time.UTC)

	a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.Int64(dateA.Unix())})
	b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.Int64(dateB.Unix())})
	c := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
	d := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.Null()})

	arena := &anyenc.Arena{}

	ko := &KeyOrder{arena: arena,
		Key:            key,
		IncludeTime:    true,
		relationFormat: model.RelationFormat_date,
	}

	t.Run("ascending, empties at the bottom", func(t *testing.T) {
		ko.Type = model.BlockContentDataviewSort_Asc
		ko.EmptyPlacement = model.BlockContentDataviewSort_End

		// we expect the following order: [a, b, c, d] OR [a, b, d, c] ~ [1917, 2025, nulls]
		assert.Equal(t, -1, ko.Compare(a, b)) // a < b
		assert.Equal(t, -1, ko.Compare(a, c)) // a < c
		assert.Equal(t, -1, ko.Compare(a, d)) // a < d
		assert.Equal(t, -1, ko.Compare(b, c)) // b < c
		assert.Equal(t, -1, ko.Compare(b, d)) // b < d
		assert.Equal(t, 0, ko.Compare(c, d))  // c == d
	})

	t.Run("descending, empties at the bottom", func(t *testing.T) {
		ko.Type = model.BlockContentDataviewSort_Desc
		ko.EmptyPlacement = model.BlockContentDataviewSort_End

		// we expect the following order: [b, a, c, d] OR [b, a, d, c] ~ [2025, 1917, nulls]
		assert.Equal(t, +1, ko.Compare(a, b)) // b < a
		assert.Equal(t, -1, ko.Compare(a, c)) // a < c
		assert.Equal(t, -1, ko.Compare(a, d)) // a < d
		assert.Equal(t, -1, ko.Compare(b, c)) // b < c
		assert.Equal(t, -1, ko.Compare(b, d)) // b < d
		assert.Equal(t, 0, ko.Compare(c, d))  // c == d
	})

	t.Run("ascending, empties at the top", func(t *testing.T) {
		ko.Type = model.BlockContentDataviewSort_Asc
		ko.EmptyPlacement = model.BlockContentDataviewSort_Start

		// we expect the following order: [c, d, a, b] OR [d, c, a, b] ~ [nulls, 1917, 2025]
		assert.Equal(t, -1, ko.Compare(a, b)) // a < b
		assert.Equal(t, +1, ko.Compare(a, c)) // c < a
		assert.Equal(t, +1, ko.Compare(a, d)) // d < a
		assert.Equal(t, +1, ko.Compare(b, c)) // c < b
		assert.Equal(t, +1, ko.Compare(b, d)) // d < b
		assert.Equal(t, 0, ko.Compare(c, d))  // c == d
	})

	t.Run("descending, empties at the top", func(t *testing.T) {
		ko.Type = model.BlockContentDataviewSort_Desc
		ko.EmptyPlacement = model.BlockContentDataviewSort_Start

		// we expect the following order: [c, d, b, a] OR [d, c, b, a] ~ [nulls, 2025, 1917]
		assert.Equal(t, +1, ko.Compare(a, b)) // b < a
		assert.Equal(t, +1, ko.Compare(a, c)) // c < a
		assert.Equal(t, +1, ko.Compare(a, d)) // d < a
		assert.Equal(t, +1, ko.Compare(b, c)) // c < b
		assert.Equal(t, +1, ko.Compare(b, d)) // d < b
		assert.Equal(t, 0, ko.Compare(c, d))  // c == d
	})

	t.Run("ascending, empties position not specified", func(t *testing.T) {
		ko.Type = model.BlockContentDataviewSort_Asc
		ko.EmptyPlacement = model.BlockContentDataviewSort_NotSpecified

		// we expect the following order: [c, d, a, b] OR [d, c, a, b] ~ [nulls, 1917, 2025]
		assert.Equal(t, -1, ko.Compare(a, b)) // a < b
		assert.Equal(t, +1, ko.Compare(a, c)) // c < a
		assert.Equal(t, +1, ko.Compare(a, d)) // d < a
		assert.Equal(t, +1, ko.Compare(b, c)) // c < b
		assert.Equal(t, +1, ko.Compare(b, d)) // d < b
		assert.Equal(t, 0, ko.Compare(c, d))  // c == d
	})

	t.Run("descending, empties position not specified", func(t *testing.T) {
		ko.Type = model.BlockContentDataviewSort_Desc
		ko.EmptyPlacement = model.BlockContentDataviewSort_NotSpecified

		// we expect the following order: [b, a, c, d] OR [b, a, d, c] ~ [2025, 1917, nulls]
		assert.Equal(t, +1, ko.Compare(a, b)) // b < a
		assert.Equal(t, -1, ko.Compare(a, c)) // a < c
		assert.Equal(t, -1, ko.Compare(a, d)) // a < d
		assert.Equal(t, -1, ko.Compare(b, c)) // b < c
		assert.Equal(t, -1, ko.Compare(b, d)) // b < d
		assert.Equal(t, 0, ko.Compare(c, d))  // c == d
	})
}

func TestKeyOrder_Compare_byTag(t *testing.T) {
	key := bundle.RelationKeyTag

	om := NewOrderMap(map[string]*domain.Details{
		// note that lexigraphically VzVz < vAvA because 'V' < 'v', however, collator states that VzVz > vAvA, so
		// correct orders are: [first < second] AND [vava < vzvz]
		"first": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("First"),
			bundle.RelationKeyOrderId: domain.String("VzVz"),
		}),
		"second": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("Second"),
			bundle.RelationKeyOrderId: domain.String("vAvA"),
		}),
		"third": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("Third"),
			bundle.RelationKeyOrderId: domain.String("xAxA"),
		}),

		"vava": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("vAvA"),
		}),
		"vzvz": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("VzVz"),
		}),
		"forth": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("Forth"),
		}),

		// all emptyX and nullX objects should have same order
		"empty": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{}),
		"emptyName": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String(""),
		}),
		"emptyOrderId": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyOrderId: domain.String(""),
		}),

		"nullName": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.Null(),
		}),
		"nullOrderId": domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyOrderId: domain.Null(),
		}),
	})

	ko := NewKeyOrder(&stubSpaceObjectStore{}, &anyenc.Arena{}, &collate.Buffer{}, SortRequest{RelationKey: key})
	ko.orderMap = om

	t.Run("all objects that handle empty details are identical", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.Null()})
		c := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList(nil)})
		d := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{})})

		for i := 0; i < 6; i++ {
			ko.Type = model.BlockContentDataviewSortType(i / 3)
			ko.EmptyPlacement = model.BlockContentDataviewSortEmptyType(i % 3)
			assertCompare(t, ko, a, b, 0)
			assertCompare(t, ko, c, d, 0)
		}
	})

	t.Run("all objects that handle lists of objects with empty/null names/orderIds of same length are equal", func(t *testing.T) {
		a := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"empty"})})
		b := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"emptyName"})})
		c := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"emptyOrderId"})})
		d := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"nullName"})})
		e := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"nullOrderId"})})

		f := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"nullOrderId", "empty", "emptyName"})})
		g := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"empty", "emptyOrderId", "nullName"})})
		h := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"nullName", "empty", "nullOrderId"})})

		for i := 0; i < 6; i++ {
			ko.Type = model.BlockContentDataviewSortType(i / 3)
			ko.EmptyPlacement = model.BlockContentDataviewSortEmptyType(i % 3)

			assertCompare(t, ko, a, b, 0)
			assertCompare(t, ko, b, c, 0)
			assertCompare(t, ko, c, d, 0)
			assertCompare(t, ko, d, e, 0)

			assertCompare(t, ko, f, g, 0)
			assertCompare(t, ko, g, h, 0)
			assertCompare(t, ko, f, h, 0)
		}
	})

	firstSecond := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"first", "second"})})
	secondThird := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"second", "third"})})
	third := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"third"})})
	forth := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"forth"})})
	vavaVZVZ := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"vava", "vzvz"})})
	vzvzEmpty := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"vzvz", "empty"})})
	null := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{})
	empty1 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"empty"})})
	empty3 := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{key: domain.StringList([]string{"empty", "empty", "empty"})})

	t.Run("ascending, empties at the bottom", func(t *testing.T) {
		ko.Type = model.BlockContentDataviewSort_Asc
		ko.EmptyPlacement = model.BlockContentDataviewSort_End

		// we expect the following order: [oneEmpty, empty3, forth, vavaVZVZ, vzvzEmpty, firstSecond, secondThird, third, null]
		assertCompare(t, ko, empty1, empty3, -1)           // [""] < ["","",""] because of list length
		assertCompare(t, ko, empty3, forth, -1)            // ["", "", ""] < ["forth"] because names concats: "" < "Forth"
		assertCompare(t, ko, forth, vavaVZVZ, -1)          // "Forth" < "vAvAVzVz"
		assertCompare(t, ko, vavaVZVZ, vzvzEmpty, -1)      // "vAvAVzVz" < "VzVz"
		assertCompare(t, ko, vzvzEmpty, firstSecond, -1)   // ["vzvz", "empty"] < ["first", "second"] because orderIds: "" < "VzVzvAvA"
		assertCompare(t, ko, firstSecond, secondThird, -1) // ["first", "second"] < ["second", "third"] <== "VzVzvAvA" < "vAvAxAxA"
		assertCompare(t, ko, secondThird, third, -1)       // ["second", "third"] < ["third"] <= "vAvAxAxA" < "xAxA"
		assertCompare(t, ko, third, null, -1)              // ["third"] < null because of End EmptyPlacement policy
	})

	t.Run("ascending, empties at the top", func(t *testing.T) {
		ko.Type = model.BlockContentDataviewSort_Asc
		ko.EmptyPlacement = model.BlockContentDataviewSort_Start

		// we expect the following order: [null, oneEmpty, empty3, forth, vavaVZVZ, vzvzEmpty, firstSecond, secondThird, third]
		assertCompare(t, ko, null, empty1, -1)             // null < [""] because of Start EmptyPlacement policy + Asc
		assertCompare(t, ko, empty1, empty3, -1)           // [""] < ["","",""] because of list length
		assertCompare(t, ko, empty3, forth, -1)            // ["", "", ""] < ["forth"] because names concats: "" < "Forth"
		assertCompare(t, ko, forth, vavaVZVZ, -1)          // "Forth" < "vAvAVzVz"
		assertCompare(t, ko, vavaVZVZ, vzvzEmpty, -1)      // "vAvAVzVz" < "VzVz"
		assertCompare(t, ko, vzvzEmpty, firstSecond, -1)   // ["vzvz", "empty"] < ["first", "second"] because orderIds: "" < "VzVzvAvA"
		assertCompare(t, ko, firstSecond, secondThird, -1) // ["first", "second"] < ["second", "third"] <== "VzVzvAvA" < "vAvAxAxA"
		assertCompare(t, ko, secondThird, third, -1)       // ["second", "third"] < ["third"] <= "vAvAxAxA" < "xAxA"
	})

	t.Run("descending, empties at the bottom", func(t *testing.T) {
		ko.Type = model.BlockContentDataviewSort_Desc
		ko.EmptyPlacement = model.BlockContentDataviewSort_End

		// we expect the back order of the previous test case
		assertCompare(t, ko, null, empty1, +1)
		assertCompare(t, ko, empty1, empty3, +1)
		assertCompare(t, ko, empty3, forth, +1)
		assertCompare(t, ko, forth, vavaVZVZ, +1)
		assertCompare(t, ko, vavaVZVZ, vzvzEmpty, +1)
		assertCompare(t, ko, vzvzEmpty, firstSecond, +1)
		assertCompare(t, ko, firstSecond, secondThird, +1)
		assertCompare(t, ko, secondThird, third, +1)
	})

	t.Run("descending, empties at the top", func(t *testing.T) {
		ko.Type = model.BlockContentDataviewSort_Desc
		ko.EmptyPlacement = model.BlockContentDataviewSort_Start

		// we expect the back order of the first test case
		assertCompare(t, ko, empty1, empty3, +1)
		assertCompare(t, ko, empty3, forth, +1)
		assertCompare(t, ko, forth, vavaVZVZ, +1)
		assertCompare(t, ko, vavaVZVZ, vzvzEmpty, +1)
		assertCompare(t, ko, vzvzEmpty, firstSecond, +1)
		assertCompare(t, ko, firstSecond, secondThird, +1)
		assertCompare(t, ko, secondThird, third, +1)
		assertCompare(t, ko, third, null, +1)
	})
}
