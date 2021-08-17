package dot

import (
	"bytes"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/converter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"io/ioutil"
)

func NewMultiConverter() converter.MultiConverter {
	g := graphviz.New()
	graph, err := g.Graph()
	if err != nil {
		return nil
	}

	return &dot{graph: graph, graphviz: g, linksByNode: map[string][]linkInfo{}, nodes: map[string]*cgraph.Node{}}
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

type dot struct {
	graph       *cgraph.Graph
	graphviz    *graphviz.Graphviz
	knownIds    []string
	fileHashes  []string
	imageHashes []string

	nodes       map[string]*cgraph.Node
	linksByNode map[string][]linkInfo
}

func (d *dot) SetKnownLinks(ids []string) converter.Converter {
	d.knownIds = ids
	return d
}

func (d *dot) FileHashes() []string {
	return d.fileHashes
}

func (d *dot) ImageHashes() []string {
	return d.imageHashes
}

func (d *dot) Add(st *state.State) error {
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
		//n.SetImage(image+".jpg")
	}

	desc := pbtypes.GetString(st.Details(), bundle.RelationKeyDescription.String())
	if desc != "" {
		n.Set("description", desc)
	}

	objType := pbtypes.GetString(st.Details(), bundle.RelationKeyType.String())
	n.Set("type", objType)

	layout := pbtypes.GetInt64(st.Details(), bundle.RelationKeyLayout.String())
	n.Set("layout", fmt.Sprintf("%d", layout))

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
			if slice.FindPos(d.knownIds, objId) == -1 {
				continue
			}
			if t != smartblock.SmartBlockTypeAnytypeProfile && t != smartblock.SmartBlockTypePage {
				continue
			}

			d.linksByNode[st.RootId()] = append(d.linksByNode[st.RootId()], linkInfo{
				target:   objId,
				edgeType: EdgeTypeRelation,
				name:     rel.Name,
				full:     rel.Description,
			})

		}
	}

	for _, depId := range st.DepSmartIds() {
		t, err := smartblock.SmartBlockTypeFromID(depId)
		if err != nil {
			continue
		}
		if slice.FindPos(d.knownIds, depId) == -1 {
			continue
		}

		if t == smartblock.SmartBlockTypeAnytypeProfile || t == smartblock.SmartBlockTypePage {
			d.linksByNode[st.RootId()] = append(d.linksByNode[st.RootId()], linkInfo{
				target:   depId,
				edgeType: EdgeTypeLink,
				name:     "", // todo: add link text
				full:     "",
			})
		}
	}

	return nil
}

func (d *dot) Convert() []byte {
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
	if err = d.graphviz.Render(d.graph, "dot", &buf); err != nil {
		return nil
	}

	b, _ := ioutil.ReadAll(&buf)
	return b
}

func (d *dot) Ext() string {
	return ".dot"
}
