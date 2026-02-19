package graphjson

import (
	"encoding/json"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

type edgeType int

const (
	EdgeTypeRelation edgeType = iota
	EdgeTypeLink
)

type Node struct {
	Id          string         `json:"id,omitempty"`
	Type        domain.TypeKey `json:"type,omitempty"`
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
	knownDocs   map[string]*domain.Details
	fileHashes  []string
	imageHashes []string
	nodes       map[string]*Node
	linksByNode map[string][]*Edge

	sbtProvider typeprovider.SmartBlockTypeProvider
}

func NewMultiConverter(
	sbtProvider typeprovider.SmartBlockTypeProvider,
) converter.MultiConverter {
	return &graphjson{
		linksByNode: map[string][]*Edge{},
		nodes:       map[string]*Node{},
		sbtProvider: sbtProvider,
	}
}

func (g *graphjson) SetKnownDocs(docs map[string]*domain.Details) converter.Converter {
	g.knownDocs = docs
	return g
}

func (g *graphjson) FileHashes() []string {
	return g.fileHashes
}

func (g *graphjson) ImageHashes() []string {
	return g.imageHashes
}

func (g *graphjson) Add(space smartblock.Space, st *state.State, fetcher relationutils.RelationFormatFetcher) error {
	n := Node{
		Id:          st.RootId(),
		Name:        st.Details().GetString(bundle.RelationKeyName),
		IconImage:   st.Details().GetString(bundle.RelationKeyIconImage),
		IconEmoji:   st.Details().GetString(bundle.RelationKeyIconEmoji),
		Description: st.Details().GetString(bundle.RelationKeyDescription),
		Type:        st.ObjectTypeKey(),
		Layout:      int(st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout)),
	}

	g.nodes[st.RootId()] = &n
	// TODO: add relations

	dependentObjectIDs := objectlink.DependentObjectIDs(st, space, fetcher, objectlink.Flags{
		Blocks:    true,
		Details:   true,
		Relations: false,
		Types:     false,
	})
	for _, depID := range dependentObjectIDs {
		t, err := g.sbtProvider.Type(st.SpaceID(), depID)
		if err != nil {
			continue
		}
		if _, ok := g.knownDocs[depID]; !ok {
			continue
		}

		if t == coresb.SmartBlockTypeAnytypeProfile || t == coresb.SmartBlockTypePage {
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

func (g *graphjson) Convert(sbType model.SmartBlockType) []byte {
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
