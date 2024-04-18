package objectgraph

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func Test_isRelationShouldBeIncludedAsEdge(t *testing.T) {

	tests := []struct {
		name string
		rel  *relationutils.Relation
		want bool
	}{
		{"creator",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyCreator)},
			false,
		},
		{"assignee",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyAssignee)},
			true,
		},
		{"cover",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyCoverId)},
			false,
		},
		{"file relation",
			&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyTrailer)},
			true,
		},
		{"custom relation",
			&relationutils.Relation{Relation: &model.Relation{Name: "custom", Format: model.RelationFormat_object}},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRelationShouldBeIncludedAsEdge(tt.rel); got != tt.want {
				t.Errorf("isRelationShouldBeIncludedAsEdge() = %v, want %v", got, tt.want)
			}
		})
	}
}
