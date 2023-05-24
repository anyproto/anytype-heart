package graphjson

import (
	"encoding/json"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func NewMultiConverter(sbtProvider typeprovider.SmartBlockTypeProvider) converter.MultiConverter {
	return &graphjson{
		linksByNode: map[string][]*Edge{},
		nodes:       map[string]*Node{},
		sbtProvider: sbtProvider,
	}
}

type edgeType int

const (
	EdgeTypeRelation edgeType = iota
	EdgeTypeLink
)

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
	knownDocs   map[string]*types.Struct
	fileHashes  []string
	imageHashes []string
	nodes       map[string]*Node
	linksByNode map[string][]*Edge
	sbtProvider typeprovider.SmartBlockTypeProvider
}

func (g *graphjson) SetKnownDocs(docs map[string]*types.Struct) converter.Converter {
	g.knownDocs = docs
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
		Id:          st.RootId(),
		Name:        pbtypes.GetString(st.Details(), bundle.RelationKeyName.String()),
		IconImage:   pbtypes.GetString(st.Details(), bundle.RelationKeyIconImage.String()),
		IconEmoji:   pbtypes.GetString(st.Details(), bundle.RelationKeyIconEmoji.String()),
		Description: pbtypes.GetString(st.Details(), bundle.RelationKeyDescription.String()),
		Type:        st.ObjectType(),
		Layout:      int(pbtypes.GetInt64(st.Details(), bundle.RelationKeyLayout.String())),
	}

	g.nodes[st.RootId()] = &n
	// TODO: rewrite to relation service
	for _, rel := range st.OldExtraRelations() {
		if rel.Format != model.RelationFormat_object {
			continue
		}
		if rel.Key == bundle.RelationKeyType.String() || rel.Key == bundle.RelationKeyId.String() {
			continue
		}

		objIds := pbtypes.GetStringList(st.Details(), rel.Key)

		for _, objId := range objIds {
			t, err := g.sbtProvider.Type(objId)
			if err != nil {
				continue
			}
			if _, ok := g.knownDocs[objId]; !ok {
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

	for _, depID := range st.DepSmartIds(true, true, false, false, false) {
		t, err := g.sbtProvider.Type(depID)
		if err != nil {
			continue
		}
		if _, ok := g.knownDocs[depID]; !ok {
			continue
		}

		if t == smartblock.SmartBlockTypeAnytypeProfile || t == smartblock.SmartBlockTypePage {
			g.linksByNode[st.RootId()] = append(g.linksByNode[st.RootId()], &Edge{
				Source:   st.RootId(),
				Target:   depID,
				EdgeType: EdgeTypeLink,
				Name:     "Link", // todo: add link text
			})
		}
	}

	return nil
}

func (g *graphjson) Convert(model.SmartBlockType) []byte {
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
