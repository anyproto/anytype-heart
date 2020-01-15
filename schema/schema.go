package schema

import (
	"log"
	"sync"

	"github.com/gogo/protobuf/jsonpb"
	tpb "github.com/textileio/go-textile/pb"
)

var schemas = map[string]*tpb.Node{}
var schemasMutex = sync.Mutex{}

func ImageNode() *tpb.Node {
	return node("image", Image)
}

func node(name, blob string) *tpb.Node {
	schemasMutex.Lock()
	defer schemasMutex.Unlock()
	if n, exists := schemas[name]; exists {
		return n
	}

	var node tpb.Node
	err := jsonpb.UnmarshalString(blob, &node)
	if err != nil {
		// this is a predefined schema and must unmarshal properly
		log.Fatalf("failed to unmarshal %s schema: %s", name, err.Error())
		return nil
	}

	schemas[name] = &node

	return &node
}
