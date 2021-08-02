// +build linux darwin
// +build !android,!ios
// +build amd64 arm64

package change

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
)

func (t *Tree) Graphviz() (data string, err error) {
	var order = make(map[string]string)
	var seq = 0
	t.Iterate(t.RootId(), func(c *Change) (isContinue bool) {
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
		if c.Snapshot != nil {
			n.SetStyle(cgraph.FilledNodeStyle)
		} else if c.HasMeta() {
			n.SetStyle(cgraph.DashedNodeStyle)
		}
		nodes[c.Id] = n
		ord := order[c.Id]
		if ord == "" {
			ord = "miss"
		}
		var chSymbs []string
		for _, chc := range c.Content {
			tp := fmt.Sprintf("%T", chc.Value)
			tp = strings.Replace(tp, "ChangeContentValueOf", "", 1)
			res := ""
			for _, ts := range tp {
				if unicode.IsUpper(ts) {
					res += string(ts)
				}
			}
			chSymbs = append(chSymbs, res)
		}

		shortId := c.Id
		if len(shortId) > 10 {
			shortId = shortId[len(c.Id)-10:]
		}
		label := fmt.Sprintf("Id: %s\nOrd: %s\nTime: %s\nChanges: %s (%d)\n",
			shortId,
			ord,
			time.Unix(c.Timestamp, 0).Format("02.01.06 15:04:05"),
			strings.Join(chSymbs, ","),
			len(c.Content),
		)
		if len(c.FileKeys) > 0 || c.Snapshot != nil {
			var l int
			if c.Snapshot != nil {
				l = len(c.Snapshot.FileKeys)
			} else {
				l = len(c.FileKeys)
			}
			label += fmt.Sprintf("FileHashes: %d\n", l)
		}
		n.SetLabel(label)
		return nil
	}
	for _, c := range t.attached {
		if err = addChange(c); err != nil {
			return
		}
	}
	for _, c := range t.unAttached {
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
	for _, c := range t.attached {
		if err = addLinks(c); err != nil {
			return
		}
	}
	for _, c := range t.unAttached {
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
