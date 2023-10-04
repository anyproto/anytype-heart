package files

import (
	"context"
	"sync"

	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

// Test dag. Copied from github.com/ipfs/go-ipld-format tests
type testDag struct {
	mu    sync.Mutex
	nodes map[string]ipld.Node
}

func newTestDag() *testDag {
	return &testDag{nodes: make(map[string]ipld.Node)}
}

func (d *testDag) Get(ctx context.Context, cid cid.Cid) (ipld.Node, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n, ok := d.nodes[cid.KeyString()]; ok {
		return n, nil
	}
	return nil, ipld.ErrNotFound{Cid: cid}
}

func (d *testDag) GetMany(ctx context.Context, cids []cid.Cid) <-chan *ipld.NodeOption {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(chan *ipld.NodeOption, len(cids))
	for _, c := range cids {
		if n, ok := d.nodes[c.KeyString()]; ok {
			out <- &ipld.NodeOption{Node: n}
		} else {
			out <- &ipld.NodeOption{Err: ipld.ErrNotFound{c}}
		}
	}
	close(out)
	return out
}

func (d *testDag) Add(ctx context.Context, node ipld.Node) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nodes[node.Cid().KeyString()] = node
	return nil
}

func (d *testDag) AddMany(ctx context.Context, nodes []ipld.Node) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, n := range nodes {
		d.nodes[n.Cid().KeyString()] = n
	}
	return nil
}

func (d *testDag) Remove(ctx context.Context, c cid.Cid) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.nodes, c.KeyString())
	return nil
}

func (d *testDag) RemoveMany(ctx context.Context, cids []cid.Cid) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, c := range cids {
		delete(d.nodes, c.KeyString())
	}
	return nil
}

var _ ipld.DAGService = new(testDag)
