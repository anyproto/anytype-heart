package graphjson

import (
	"encoding/json"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type edgeType int

const (
	EdgeTypeRelation edgeType = iota
	EdgeTypeLink
)

type Node struct {
	Id          string         `json:"id,omitempty"`
	Type        bundle.TypeKey `json:"type,omitempty"`
	Name        string         `json:"name,omitempty"`
	Layout      int            `json:"layout,omitempty"`
	Description string         `json:"description,omitempty"`
	IconImage   string         `json:"iconImage,omitempty"`
	IconEmoji   string         `json:"iconEmoji,omitempty"`
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

	sbtProvider         typeprovider.SmartBlockTypeProvider
	systemObjectService system_object.Service
}

func NewMultiConverter(
	sbtProvider typeprovider.SmartBlockTypeProvider,
	systemObjectService system_object.Service,
) converter.MultiConverter {
	return &graphjson{
		linksByNode:         map[string][]*Edge{},
		nodes:               map[string]*Node{},
		sbtProvider:         sbtProvider,
		systemObjectService: systemObjectService,
	}
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
		Type:        st.ObjectTypeKey(),
		Layout:      int(pbtypes.GetInt64(st.Details(), bundle.RelationKeyLayout.String())),
	}

	g.nodes[st.RootId()] = &n
	// TODO: add relations

	dependentObjectIDs := objectlink.DependentObjectIDs(st, g.systemObjectService, true, true, false, false, false)
	for _, depID := range dependentObjectIDs {
		t, err := g.sbtProvider.Type(st.SpaceID(), depID)
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
