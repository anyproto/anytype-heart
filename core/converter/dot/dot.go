//go:build !gomobile && !windows && !nographviz && cgo
// +build !gomobile,!windows,!nographviz,cgo

package dot

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/converter"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	ExportFormatDOT = graphviz.XDOT
	ExportFormatSVG = graphviz.SVG
)

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

type dot struct {
	graph        *cgraph.Graph
	graphviz     *graphviz.Graphviz
	knownDocs    map[string]*types.Struct
	fileHashes   []string
	imageHashes  []string
	exportFormat graphviz.Format
	nodes        map[string]*cgraph.Node
	linksByNode  map[string][]linkInfo
	sbtProvider  typeprovider.SmartBlockTypeProvider
}

func NewMultiConverter(
	format graphviz.Format,
	sbtProvider typeprovider.SmartBlockTypeProvider,
) converter.MultiConverter {
	g := graphviz.New()
	graph, err := g.Graph()
	if err != nil {
		return nil
	}

	return &dot{
		graph:        graph,
		graphviz:     g,
		exportFormat: format,
		linksByNode:  map[string][]linkInfo{},
		nodes:        map[string]*cgraph.Node{},
		sbtProvider:  sbtProvider,
	}
}

func (d *dot) SetKnownDocs(docs map[string]*types.Struct) converter.Converter {
	d.knownDocs = docs
	return d
}

func (d *dot) FileHashes() []string {
	return d.fileHashes
}

func (d *dot) ImageHashes() []string {
	return d.imageHashes
}

func (d *dot) Add(space smartblock.Space, st *state.State) error {
	n, e := d.graph.CreateNode(st.RootId())
	if e != nil {
		return e
	}
	d.nodes[st.RootId()] = n
	n.SetStyle(cgraph.FilledNodeStyle)
	n.SetLabel(pbtypes.GetString(st.Details(), bundle.RelationKeyName.String()))
	image := pbtypes.GetString(st.Details(), bundle.RelationKeyIconImage.String())
	if image != "" {
		n.Set("iconImage", image)
		// n.SetImage(image+".jpg")
	}

	iconEmoji := pbtypes.GetString(st.Details(), bundle.RelationKeyIconEmoji.String())
	if iconEmoji != "" {
		n.Set("iconEmoji", iconEmoji)
	}

	desc := pbtypes.GetString(st.Details(), bundle.RelationKeyDescription.String())
	if desc != "" {
		n.Set("description", desc)
	}

	n.Set("type", string(st.ObjectTypeKey()))
	layout := pbtypes.GetInt64(st.Details(), bundle.RelationKeyLayout.String())
	n.Set("layout", fmt.Sprintf("%d", layout))

	// TODO: add relations

	dependentObjectIDs := objectlink.DependentObjectIDs(st, space, true, true, false, false, false)
	for _, depID := range dependentObjectIDs {
		t, err := d.sbtProvider.Type(st.SpaceID(), depID)
		if err != nil {
			continue
		}
		if _, ok := d.knownDocs[depID]; !ok {
			continue
		}

		if t == coresb.SmartBlockTypeAnytypeProfile || t == coresb.SmartBlockTypePage {
			d.linksByNode[st.RootId()] = append(d.linksByNode[st.RootId()], linkInfo{
				target:   depID,
				edgeType: EdgeTypeLink,
				name:     "", // todo: add link text
				full:     "",
			})
		}
	}

	return nil
}

func (d *dot) Convert(sbType model.SmartBlockType) []byte {
	var err error
	for id, links := range d.linksByNode {
		source, exists := d.nodes[id]
		if !exists {
			continue
		}

		var e *cgraph.Edge
		for _, link := range links {
			target, exists := d.nodes[link.target]
			if !exists {
				continue
			}
			e, err = d.graph.CreateEdge("", source, target)
			if err != nil {
				return nil
			}
			e.SetLabel(link.name)
			e.SetTooltip(link.full)

			if link.edgeType == EdgeTypeLink {
				e.SetStyle(cgraph.DashedEdgeStyle)
			}
		}
	}

	var buf bytes.Buffer
	if err = d.graphviz.Render(d.graph, d.exportFormat, &buf); err != nil {
		return nil
	}

	b, _ := ioutil.ReadAll(&buf)
	return b
}

func (d *dot) Ext() string {
	if d.exportFormat == graphviz.SVG {
		return ".svg"
	}
	return ".dot"
}
