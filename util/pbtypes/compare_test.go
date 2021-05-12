package pbtypes

import (
	"reflect"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
)

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
