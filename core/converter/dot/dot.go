//go:build !gomobile && !windows && !nographviz && cgo
// +build !gomobile,!windows,!nographviz,cgo

package dot

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"

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
	ctx          context.Context
	graph        *cgraph.Graph
	graphviz     *graphviz.Graphviz
	knownDocs    map[string]*domain.Details
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
	ctx := context.Background()
	g, err := graphviz.New(ctx)
	if err != nil {
		return nil
	}
	graph, err := g.Graph()
	if err != nil {
		return nil
	}

	return &dot{
		ctx:          ctx,
		graph:        graph,
		graphviz:     g,
		exportFormat: format,
		linksByNode:  map[string][]linkInfo{},
		nodes:        map[string]*cgraph.Node{},
		sbtProvider:  sbtProvider,
	}
}

func (d *dot) SetKnownDocs(docs map[string]*domain.Details) converter.Converter {
	d.knownDocs = docs
	return d
}

func (d *dot) FileHashes() []string {
	return d.fileHashes
}

func (d *dot) ImageHashes() []string {
	return d.imageHashes
}

func (d *dot) Add(space smartblock.Space, st *state.State, fetcher relationutils.RelationFormatFetcher) error {
	n, e := d.graph.CreateNodeByName(st.RootId())
	if e != nil {
		return e
	}
	d.nodes[st.RootId()] = n
	n.SetStyle(cgraph.FilledNodeStyle)
	n.SetLabel(st.Details().GetString(bundle.RelationKeyName))
	image := st.Details().GetString(bundle.RelationKeyIconImage)
	if image != "" {
		n.Set("iconImage", image)
		// n.SetImage(image+".jpg")
	}

	iconEmoji := st.Details().GetString(bundle.RelationKeyIconEmoji)
	if iconEmoji != "" {
		n.Set("iconEmoji", iconEmoji)
	}

	desc := st.Details().GetString(bundle.RelationKeyDescription)
	if desc != "" {
		n.Set("description", desc)
	}

	n.Set("type", string(st.ObjectTypeKey()))
	layout := st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout)
	n.Set("layout", fmt.Sprintf("%d", layout))

	// TODO: add relations

	dependentObjectIDs := objectlink.DependentObjectIDs(st, space, fetcher, objectlink.Flags{
		Blocks:    true,
		Details:   true,
		Relations: false,
		Types:     false,
	})
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
			e, err = d.graph.CreateEdgeByName("", source, target)
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
	if err = d.graphviz.Render(d.ctx, d.graph, d.exportFormat, &buf); err != nil {
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
