package graphjson

import (
	"encoding/json"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

func NewMultiConverter() converter.MultiConverter {
	return &graphjson{linksByNode: map[string][]*Edge{}, nodes: map[string]*Node{}}
}

type edgeType int

const (
	EdgeTypeRelation edgeType = iota
	EdgeTypeLink
)

type linkInfo struct {
	target   string
	edgeType edgeType
	name     string
	full     string
}

type Node struct {
	Id          string `json:"id,omitempty"`
	Type        string `json:"type,omitempty"`
	Name        string `json:"name,omitempty"`
	Layout      int    `json:"layout,omitempty"`
	Description string `json:"description,omitempty"`
	IconImage   string `json:"iconImage,omitempty"`
	IconEmoji   string `json:"iconEmoji,omitempty"`
}

type Edge struct {
	Source   string   `json:"source,omitempty"`
	Target   string   `json:"target,omitempty"`
	Name     string   `json:"name,omitempty"`
	EdgeType edgeType `json:"type,omitempty"`
}

type Graph struct {
	Nodes []*Node `json:"nodes,omitempty"`
	Edges []*Edge `json:"edges,omitempty"`
}

type graphjson struct {
	knownIds    []string
	fileHashes  []string
	imageHashes []string
	nodes       map[string]*Node
	linksByNode map[string][]*Edge
}

func (g *graphjson) SetKnownLinks(ids []string) converter.Converter {
	g.knownIds = ids
	return g
}

func (g *graphjson) FileHashes() []string {
	return g.fileHashes
}

func (g *graphjson) ImageHashes() []string {
	return g.imageHashes
}

func (g *graphjson) Add(st *state.State) error {
	n := Node{
		Name:        pbtypes.GetString(st.Details(), bundle.RelationKeyName.String()),
		IconImage:   pbtypes.GetString(st.Details(), bundle.RelationKeyIconImage.String()),
		IconEmoji:   pbtypes.GetString(st.Details(), bundle.RelationKeyIconEmoji.String()),
		Description: pbtypes.GetString(st.Details(), bundle.RelationKeyDescription.String()),
		Type:        st.ObjectType(),
		Layout:      int(pbtypes.GetInt64(st.Details(), bundle.RelationKeyLayout.String())),
	}

	g.nodes[st.RootId()] = &n
	for _, rel := range st.ExtraRelations() {
		if rel.Format != model.RelationFormat_object {
			continue
		}
		if rel.Key == bundle.RelationKeyType.String() || rel.Key == bundle.RelationKeyId.String() {
			continue
		}

		objIds := pbtypes.GetStringList(st.Details(), rel.Key)

		for _, objId := range objIds {
			t, err := smartblock.SmartBlockTypeFromID(objId)
			if err != nil {
				continue
			}
			if slice.FindPos(g.knownIds, objId) == -1 {
				continue
			}
			if t != smartblock.SmartBlockTypeAnytypeProfile && t != smartblock.SmartBlockTypePage {
				continue
			}

			g.linksByNode[st.RootId()] = append(g.linksByNode[st.RootId()], &Edge{
				Source:   st.RootId(),
				Target:   objId,
				EdgeType: EdgeTypeRelation,
				Name:     rel.Name,
			})

		}
	}

	for _, depId := range st.DepSmartIds() {
		t, err := smartblock.SmartBlockTypeFromID(depId)
		if err != nil {
			continue
		}
		if slice.FindPos(g.knownIds, depId) == -1 {
			continue
		}

		if t == smartblock.SmartBlockTypeAnytypeProfile || t == smartblock.SmartBlockTypePage {
			g.linksByNode[st.RootId()] = append(g.linksByNode[st.RootId()], &Edge{
				Source:   st.RootId(),
				Target:   depId,
				EdgeType: EdgeTypeLink,
				Name:     "Link", // todo: add link text
			})
		}
	}

	return nil
}

func (g *graphjson) Convert() []byte {
	d := &Graph{
		Nodes: make([]*Node, 0, len(g.nodes)),
		Edges: make([]*Edge, 0, len(g.linksByNode)),
	}

	for _, node := range g.nodes {
		d.Nodes = append(d.Nodes, node)
	}

	for _, links := range g.linksByNode {
		for _, link := range links {
			if _, exists := g.nodes[link.Target]; !exists {
				continue
			}

			d.Edges = append(d.Edges, link)
		}
	}

	b, _ := json.Marshal(d)
	return b
}

func (g *graphjson) Ext() string {
	return ".json"
}
