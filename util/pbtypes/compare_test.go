package pbtypes

import (
	"reflect"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestStructMerge(t *testing.T) {
	t.Run("both inputs nil", func(t *testing.T) {
		got := StructMerge(nil, nil, false)
		if got != nil {
			t.Errorf("StructMerge(nil, nil, false) = %v, want %v", got, nil)
		}
	})

	t.Run("first input nil or has nil fields", func(t *testing.T) {
		st2 := &types.Struct{Fields: map[string]*types.Value{"key2": {}}}
		got := StructMerge(nil, st2, false)
		if !reflect.DeepEqual(got, CopyStruct(st2, false)) {
			t.Errorf("StructMerge(nil, st2, false) did not behave like a copy of st2")
		}

		st1WithNilFields := &types.Struct{Fields: nil}
		got = StructMerge(st1WithNilFields, st2, false)
		if !reflect.DeepEqual(got, CopyStruct(st2, false)) {
			t.Errorf("StructMerge(st1WithNilFields, st2, false) did not behave like a copy of st2")
		}
	})

	t.Run("second input nil or has nil fields", func(t *testing.T) {
		st1 := &types.Struct{Fields: map[string]*types.Value{"key1": {}}}
		got := StructMerge(st1, nil, false)
		if !reflect.DeepEqual(got, CopyStruct(st1, false)) {
			t.Errorf("StructMerge(st1, nil, false) did not behave like a copy of st1")
		}

		st2WithNilFields := &types.Struct{Fields: nil}
		got = StructMerge(st1, st2WithNilFields, false)
		if !reflect.DeepEqual(got, CopyStruct(st1, false)) {
			t.Errorf("StructMerge(st1, st2WithNilFields, false) did not behave like a copy of st1")
		}
	})

	t.Run("non-empty structs without copying values", func(t *testing.T) {
		st1 := &types.Struct{Fields: map[string]*types.Value{"key1": {}}}
		st2 := &types.Struct{Fields: map[string]*types.Value{"key2": {}}}
		got := StructMerge(st1, st2, false)
		if len(got.Fields) != 2 || got.Fields["key1"] == nil || got.Fields["key2"] == nil {
			t.Errorf("StructMerge did not correctly merge fields without copying values")
		}
	})

	t.Run("non-empty structs with copying values", func(t *testing.T) {
		st1 := &types.Struct{Fields: map[string]*types.Value{"key1": String("1")}}
		st2 := &types.Struct{Fields: map[string]*types.Value{"key2": String("2")}}
		got := StructMerge(st1, st2, true)

		require.NotSame(t, got.Fields["key2"], st2.Fields["key2"])
		require.Len(t, got.Fields, 2)

	})

	t.Run("field present in both structs", func(t *testing.T) {
		st1 := &types.Struct{Fields: map[string]*types.Value{"key": String("1")}}
		st2 := &types.Struct{Fields: map[string]*types.Value{"key": String("2")}}
		gotWithoutCopy := StructMerge(st1, st2, false)
		require.Same(t, gotWithoutCopy.Fields["key"], st2.Fields["key"])

		gotWithCopy := StructMerge(st1, st2, true)
		require.NotSame(t, gotWithCopy.Fields["key"], st2.Fields["key"])
		require.Equal(t, gotWithCopy.Fields["key"], st2.Fields["key"])

		require.Len(t, gotWithCopy.Fields, 1)
	})
}

func TestStructDiff(t *testing.T) {
	type args struct {
		st1 *types.Struct
		st2 *types.Struct
	}
	tests := []struct {
		name string
		args args
		want *types.Struct
	}{
		{"both nil",
			args{nil, nil},
			nil,
		},
		{"equal",
			args{
				&types.Struct{
					Fields: map[string]*types.Value{
						"k1": String("v1"),
					},
				},
				&types.Struct{
					Fields: map[string]*types.Value{
						"k1": String("v1"),
					}},
			},
			nil,
		},
		{"nil st1", args{
			nil,
			&types.Struct{
				Fields: map[string]*types.Value{
					"k1": String("v1"),
				},
			},
		}, &types.Struct{
			Fields: map[string]*types.Value{
				"k1": String("v1"),
			},
		}},
		{"nil map st1", args{
			&types.Struct{
				Fields: nil,
			},
			&types.Struct{
				Fields: map[string]*types.Value{
					"k1": String("v1"),
				},
			},
		}, &types.Struct{
			Fields: map[string]*types.Value{
				"k1": String("v1"),
			},
		}},
		{"empty map st1", args{
			&types.Struct{
				Fields: map[string]*types.Value{},
			},
			&types.Struct{
				Fields: map[string]*types.Value{
					"k1": String("v1"),
				},
			},
		}, &types.Struct{
			Fields: map[string]*types.Value{
				"k1": String("v1"),
			},
		}},
		{"nil st2", args{
			&types.Struct{
				Fields: map[string]*types.Value{
					"k1": String("v1"),
				},
			},
			nil,
		}, &types.Struct{
			Fields: map[string]*types.Value{
				"k1": nil,
			},
		}},
		{"nil map st2", args{
			&types.Struct{
				Fields: map[string]*types.Value{
					"k1": String("v1"),
				},
			},
			&types.Struct{
				Fields: nil,
			},
		}, &types.Struct{
			Fields: map[string]*types.Value{
				"k1": nil,
			},
		}},
		{"empty map st2", args{
			&types.Struct{
				Fields: map[string]*types.Value{
					"k1": String("v1"),
				},
			},
			&types.Struct{
				Fields: map[string]*types.Value{},
			},
		}, &types.Struct{
			Fields: map[string]*types.Value{
				"k1": nil,
			},
		}},
		{"complex", args{
			&types.Struct{
				Fields: map[string]*types.Value{
					"k1": String("v1"),
					"k2": String("v2"),
					"k3": String("v3"),
				},
			},
			&types.Struct{
				Fields: map[string]*types.Value{
					"k1": String("v1"),
					"k3": String("v3_"),
				},
			},
		}, &types.Struct{
			Fields: map[string]*types.Value{
				"k2": nil,
				"k3": String("v3_"),
			},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StructDiff(tt.args.st1, tt.args.st2); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StructDiff() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRelationsDiff(t *testing.T) {
	type args struct {
		rels1 []*model.Relation
		rels2 []*model.Relation
	}
	tests := []struct {
		name        string
		args        args
		wantAdded   []*model.Relation
		wantUpdated []*model.Relation
		wantRemoved []string
	}{
		{"complex",
			args{
				[]*model.Relation{{Key: "k0", Format: model.RelationFormat_longtext}, {Key: "k1", Format: model.RelationFormat_longtext}, {Key: "k2", Format: model.RelationFormat_longtext}},
				[]*model.Relation{{Key: "k1", Format: model.RelationFormat_longtext}, {Key: "k2", Format: model.RelationFormat_tag}, {Key: "k3", Format: model.RelationFormat_object}},
			},
			[]*model.Relation{{Key: "k3", Format: model.RelationFormat_object}},
			[]*model.Relation{{Key: "k2", Format: model.RelationFormat_tag}},
			[]string{"k0"},
		},
		{"both empty",
			args{
				[]*model.Relation{},
				[]*model.Relation{},
			},
			nil,
			nil,
			nil,
		},
		{"both nil",
			args{
				nil,
				nil,
			},
			nil,
			nil,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdded, gotUpdated, gotRemoved := RelationsDiff(tt.args.rels1, tt.args.rels2)
			if !reflect.DeepEqual(gotAdded, tt.wantAdded) {
				t.Errorf("RelationsDiff() gotAdded = %v, want %v", gotAdded, tt.wantAdded)
			}
			if !reflect.DeepEqual(gotUpdated, tt.wantUpdated) {
				t.Errorf("RelationsDiff() gotUpdated = %v, want %v", gotUpdated, tt.wantUpdated)
			}
			if !reflect.DeepEqual(gotRemoved, tt.wantRemoved) {
				t.Errorf("RelationsDiff() gotRemoved = %v, want %v", gotRemoved, tt.wantRemoved)
			}
		})
	}
}
