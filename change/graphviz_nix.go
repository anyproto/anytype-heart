// +build linux darwin
// +build !android,!ios
// +build amd64

package change

import (
	"bytes"
	"fmt"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
)

func (tr *Tree) Graphviz() (data string, err error) {
	var order = make(map[string]string)
	var seq = 0
	tr.Iterate(tr.RootId(), func(c *Change) (isContinue bool) {
		v := order[c.Id]
		if v == "" {
			order[c.Id] = fmt.Sprint(seq)
		} else {
			order[c.Id] = fmt.Sprintf("%s,%d", v, seq)
		}
		seq++
		return true
	})
	g := graphviz.New()
	defer g.Close()
	graph, err := g.Graph()
	if err != nil {
		return
	}
	defer func() {
		err = graph.Close()
	}()
	var nodes = make(map[string]*cgraph.Node)
	var addChange = func(c *Change) error {
		n, e := graph.CreateNode(c.Id)
		if e != nil {
			return e
		}
		nodes[c.Id] = n
		ord := order[c.Id]
		if ord == "" {
			ord = "miss"
		}
		n.SetLabel(fmt.Sprintf("%s: %s", c.Id, ord))
		return nil
	}
	for _, c := range tr.attached {
		if err = addChange(c); err != nil {
			return
		}
	}
	for _, c := range tr.unAttached {
		if err = addChange(c); err != nil {
			return
		}
	}
	var getNode = func(id string) (*cgraph.Node, error) {
		if n, ok := nodes[id]; ok {
			return n, nil
		}
		n, err := graph.CreateNode(fmt.Sprintf("%s: not in tree", id))
		if err != nil {
			return nil, err
		}
		nodes[id] = n
		return n, nil
	}
	var addLinks = func(c *Change) error {
		for _, prevId := range c.PreviousIds {
			self, e := getNode(c.Id)
			if e != nil {
				return e
			}
			prev, e := getNode(prevId)
			if e != nil {
				return e
			}
			_, e = graph.CreateEdge("", self, prev)
			if e != nil {
				return e
			}
		}
		return nil
	}
	for _, c := range tr.attached {
		if err = addLinks(c); err != nil {
			return
		}
	}
	for _, c := range tr.unAttached {
		if err = addLinks(c); err != nil {
			return
		}
	}
	var buf bytes.Buffer
	if err = g.Render(graph, "dot", &buf); err != nil {
		return
	}
	return buf.String(), nil
}
