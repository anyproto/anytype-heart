package domain

import (
	"reflect"
	"testing"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestStructDiff(t *testing.T) {
	type args struct {
		st1 *Details
		st2 *Details
	}
	tests := []struct {
		name     string
		args     args
		wantDiff *Details
		wantKeys []RelationKey
	}{
		{"both nil",
			args{nil, nil},
			nil, nil,
		},
		{"equal",
			args{
				NewDetailsFromMap(map[RelationKey]Value{
					"k1": String("v1"),
				}),
				NewDetailsFromMap(map[RelationKey]Value{
					"k1": String("v1"),
				}),
			},
			nil, nil,
		},
		{"nil st1", args{
			nil,
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
		}, NewDetailsFromMap(map[RelationKey]Value{
			"k1": String("v1"),
		}), nil,
		},
		{"nil map st1", args{
			NewDetails(),
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
		}, NewDetailsFromMap(map[RelationKey]Value{
			"k1": String("v1"),
		}), nil,
		},
		{"empty map st1", args{
			NewDetailsFromMap(map[RelationKey]Value{}),
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
		}, NewDetailsFromMap(map[RelationKey]Value{
			"k1": String("v1"),
		}), nil,
		},
		{"nil st2", args{
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
			nil,
		}, nil, []RelationKey{"k1"},
		},
		{"nil map st2", args{
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
			NewDetails(),
		},
			nil, []RelationKey{"k1"},
		},
		{"empty map st2", args{
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
			NewDetailsFromMap(map[RelationKey]Value{})},
			nil, []RelationKey{"k1"},
		},
		{"complex", args{
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
				"k2": String("v2"),
				"k3": String("v3"),
			}),
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
				"k3": String("v3_"),
			}),
		}, NewDetailsFromMap(map[RelationKey]Value{
			"k3": String("v3_"),
		}), []RelationKey{"k2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, keys := StructDiff(tt.args.st1, tt.args.st2)
			assert.True(t, reflect.DeepEqual(diff, tt.wantDiff))
			assert.True(t, reflect.DeepEqual(keys, tt.wantKeys))
		})
	}
}

func TestNewDetailsFromAnyEnc(t *testing.T) {
	arena := &anyenc.Arena{}

	t.Run("empty", func(t *testing.T) {
		val := anyenc.MustParseJson(`{}`)

		got, err := NewDetailsFromAnyEnc(val)
		require.NoError(t, err)

		want := NewDetails()
		assert.Equal(t, want, got)

		gotVal := got.ToAnyEnc(arena)
		diff, err := pbtypes.DiffAnyEnc(val, gotVal)
		require.NoError(t, err)
		assert.Empty(t, diff)
	})

	t.Run("all types", func(t *testing.T) {
		val := anyenc.MustParseJson(`
			{
				"key1": "value1",
				"key2": 123,
				"key3": 123.456,
				"key4": true,
				"key5": false,
				"key6": null,
				"key7": [1,2,3],
				"key8":["foo","bar"],
				"key9": {"nestedKey1": "value1", "nestedKey2": 123}
		}`)

		got, err := NewDetailsFromAnyEnc(val)
		require.NoError(t, err)

		want := NewDetailsFromMap(map[RelationKey]Value{
			"key1": String("value1"),
			"key2": Int64(123),
			"key3": Float64(123.456),
			"key4": Bool(true),
			"key5": Bool(false),
			"key6": Null(),
			"key7": Int64List([]int64{1, 2, 3}),
			"key8": StringList([]string{"foo", "bar"}),
			"key9": Null(),
		})
		assert.Equal(t, want, got)

		gotVal := got.ToAnyEnc(arena)
		diff, err := pbtypes.DiffAnyEnc(val, gotVal)
		require.NoError(t, err)

		// We don't yet support converting nested objects from AnyEnc to proto
		assert.Equal(t, []pbtypes.AnyEncDiff{
			{
				Type:  pbtypes.AnyEncDiffTypeUpdate,
				Key:   "key9",
				Value: arena.NewNull(),
			},
		}, diff)
	})
}
