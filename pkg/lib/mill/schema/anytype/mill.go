package anytype

import (
	"fmt"
	"sync"

	"github.com/gogo/protobuf/jsonpb"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/mill"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
)

var log = logging.Logger("anytype-core-mill")

func GetMill(id string, opts map[string]string) (mill.Mill, error) {
	switch id {
	case "/blob":
		return &mill.Blob{}, nil
	case "/image/resize":
		width := opts["width"]
		if width == "" {
			return nil, fmt.Errorf("missing width")
		}
		quality := opts["quality"]
		if quality == "" {
			quality = "75"
		}
		return &mill.ImageResize{
			Opts: mill.ImageResizeOpts{
				Width:   width,
				Quality: quality,
			},
		}, nil
	case "/image/exif":
		return &mill.ImageExif{}, nil
	case "/json":
		return &mill.Json{}, nil
	default:
		return nil, nil
	}
}

var schemas = map[string]*storage.Node{}
var schemasMutex = sync.Mutex{}

func ImageNode() *storage.Node {
	return node("image", Image)
}

func node(name, blob string) *storage.Node {
	schemasMutex.Lock()
	defer schemasMutex.Unlock()
	if n, exists := schemas[name]; exists {
		return n
	}

	var node storage.Node
	err := jsonpb.UnmarshalString(blob, &node)
	if err != nil {
		// this is a predefined schema and must unmarshal properly
		log.Fatalf("failed to unmarshal %s schema: %s", name, err.Error())
		return nil
	}

	schemas[name] = &node

	return &node
}
