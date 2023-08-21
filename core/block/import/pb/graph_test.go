package pb

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/stretchr/testify/assert"
)

func Test_findBidirectionalLinks(t *testing.T) {
	var tests = []struct {
		name  string
		given converter.LinksGraph
		want  map[string][]string
		want1 converter.LinksGraph
	}{
		{
			name: "Graph without bidirectional links",
			given: map[string]converter.Neighbors{
				"object1": map[string]struct{}{"object2": {}, "object3": {}},
				"object2": map[string]struct{}{"object3": {}, "object4": {}},
			},
			want: map[string][]string{},
			want1: map[string]converter.Neighbors{
				"object1": map[string]struct{}{"object2": {}, "object3": {}},
				"object2": map[string]struct{}{"object3": {}, "object4": {}},
			},
		},
		{
			name: "Graph with bidirectional links - simple case",
			given: map[string]converter.Neighbors{
				"object1": map[string]struct{}{"object2": {}},
				"object2": map[string]struct{}{"object1": {}},
			},
			want:  map[string][]string{"object1": {"object2"}, "object2": {"object1"}},
			want1: map[string]converter.Neighbors{},
		},
		{
			name: "Graph with multiple bidirectional links",
			given: map[string]converter.Neighbors{
				"object1": map[string]struct{}{"object2": {}, "object3": {}},
				"object2": map[string]struct{}{"object1": {}},
				"object3": map[string]struct{}{"object1": {}},
			},
			want:  map[string][]string{"object1": {"object2", "object3"}, "object2": {"object1"}, "object3": {"object1"}},
			want1: map[string]converter.Neighbors{},
		},
		{
			name: "Graph with multiple bidirectional links and ordinary links",
			given: map[string]converter.Neighbors{
				"object1": map[string]struct{}{"object2": {}, "object3": {}},
				"object2": map[string]struct{}{"object1": {}},
				"object3": map[string]struct{}{"object2": {}},
				"object4": map[string]struct{}{"object5": {}},
				"object5": map[string]struct{}{"object3": {}},
			},
			want: map[string][]string{"object1": {"object2"}, "object2": {"object1"}},
			want1: map[string]converter.Neighbors{
				"object3": map[string]struct{}{"object2": {}},
				"object4": map[string]struct{}{"object5": {}},
				"object5": map[string]struct{}{"object3": {}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bidirectionalLinks, graphWithoutBidirectionalLinks := findBidirectionalLinks(tt.given)
			assert.Equal(t, tt.want, bidirectionalLinks)
			assert.Equal(t, tt.want1, graphWithoutBidirectionalLinks)
		})
	}
}

func Test_findBidirectionalLinksWithoutInboundLinks(t *testing.T) {
	t.Run("No bidirectional links - no objects to add to root collection", func(t *testing.T) {
		// given
		graphWithoutBidirectionalLinks := map[string]converter.Neighbors{
			"object3": map[string]struct{}{"object2": {}},
			"object4": map[string]struct{}{"object5": {}},
			"object5": map[string]struct{}{"object3": {}},
		}

		// when
		rootObjects := findBidirectionalLinksWithoutInboundLinks(graphWithoutBidirectionalLinks, map[string][]string{})

		// then
		assert.Len(t, rootObjects, 0)
	})
	t.Run("2 objects are bidirectionally linked without other inbound links - return 2 objects to add to root collection", func(t *testing.T) {
		// given
		graphWithoutBidirectionalLinks := map[string]converter.Neighbors{
			"object4": map[string]struct{}{"object5": {}},
			"object5": map[string]struct{}{"object3": {}},
		}
		bidirectionalLinks := map[string][]string{
			"object1": {"object2"},
			"object2": {"object1"},
		}

		// when
		rootObjects := findBidirectionalLinksWithoutInboundLinks(graphWithoutBidirectionalLinks, bidirectionalLinks)

		//then
		assert.Len(t, rootObjects, 2)
		assert.Contains(t, rootObjects, "object1")
		assert.Contains(t, rootObjects, "object2")
	})
	t.Run("2 objects are bidirectionally linked, 1 object have inbound link - no objects to return", func(t *testing.T) {
		// given
		graphWithoutBidirectionalLinks := map[string]converter.Neighbors{
			"object3": map[string]struct{}{"object2": {}},
		}
		bidirectionalLinks := map[string][]string{
			"object1": {"object2"},
			"object2": {"object1"},
		}

		// when
		rootObjects := findBidirectionalLinksWithoutInboundLinks(graphWithoutBidirectionalLinks, bidirectionalLinks)

		// then
		assert.Len(t, rootObjects, 0)
	})
	t.Run("2 objects are bidirectionally linked, 1 object have other bidirectional link - return 3 objects to add to root collection", func(t *testing.T) {
		// given
		graphWithoutBidirectionalLinks := map[string]converter.Neighbors{}
		bidirectionalLinks := map[string][]string{
			"object1": {"object2"},
			"object2": {"object1", "object3"},
			"object3": {"object2"},
		}

		//when
		rootObjects := findBidirectionalLinksWithoutInboundLinks(graphWithoutBidirectionalLinks, bidirectionalLinks)

		// then
		assert.Len(t, rootObjects, 3)
		assert.Contains(t, rootObjects, "object1")
		assert.Contains(t, rootObjects, "object2")
		assert.Contains(t, rootObjects, "object3")
	})
	t.Run("2 objects are bidirectionally linked, 1 object have other bidirectional link and 1 object have outbound link - no object to return", func(t *testing.T) {
		// given
		graphWithoutBidirectionalLinks := map[string]converter.Neighbors{
			"object4": map[string]struct{}{"object1": {}},
		}
		bidirectionalLinks := map[string][]string{
			"object1": {"object2"},
			"object2": {"object1", "object3"},
			"object3": {"object2"},
		}

		// when
		rootObjects := findBidirectionalLinksWithoutInboundLinks(graphWithoutBidirectionalLinks, bidirectionalLinks)

		// then
		assert.Len(t, rootObjects, 0)
	})
}

func Test_findObjectsWithoutAnyLinks(t *testing.T) {
	type args struct {
		objectsLinks converter.LinksGraph
		objects      []string
	}
	tests := []struct {
		name  string
		given args
		want  []string
	}{
		{
			name: "No links to return",
			given: args{
				objectsLinks: map[string]converter.Neighbors{},
				objects:      nil,
			},
			want: nil,
		},
		{
			name: "Object1 has links to Object2, Object3 - return Object1",
			given: args{
				objectsLinks: map[string]converter.Neighbors{
					"object1": map[string]struct{}{"object2": {}, "object3": {}},
				},
				objects: []string{"object1", "object2", "object3"},
			},
			want: []string{"object1"},
		},
		{
			name: "Object1 has links to Object2, Object3, Object2 has links to Object1, Object3 - no objects to return",
			given: args{
				objectsLinks: map[string]converter.Neighbors{
					"object1": map[string]struct{}{"object2": {}, "object3": {}},
					"object2": map[string]struct{}{"object1": {}, "object3": {}},
				},
				objects: []string{"object1", "object2", "object3"},
			},
			want: nil,
		},
		{
			name: "Object1 has link to Object2, Object2 has link to Object3, Object4 without links - return Object1 и Object4",
			given: args{
				objectsLinks: map[string]converter.Neighbors{
					"object1": map[string]struct{}{"object2": {}},
					"object2": map[string]struct{}{"object3": {}},
				},
				objects: []string{"object1", "object2", "object3", "object4"},
			},
			want: []string{"object1", "object4"},
		},
		{
			name: "Object1, Object2, Object3 have no links - return Object1, Object2, Object3",
			given: args{
				objects: []string{"object1", "object2", "object3"},
			},
			want: []string{"object1", "object2", "object3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, findObjectsWithoutInboundLinks(tt.given.objectsLinks, tt.given.objects))
		})
	}
}
