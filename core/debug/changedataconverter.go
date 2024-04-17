package debug

import (
	"fmt"
	"sync"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/anonymize"
)

const poolSize = 4096

var bytesPool = sync.Pool{New: func() any { return make([]byte, poolSize) }}

type changeDataConverter struct {
	anonymize bool
}

func (c *changeDataConverter) Unmarshall(decrypted []byte) (res any, err error) {
	ch := &pb.Change{}
	err = proto.Unmarshal(decrypted, ch)
	if err == nil {
		return ch, nil
	}

	buf := bytesPool.Get().([]byte)[:0]
	defer bytesPool.Put(buf)

	// suppose we meet snappy-encoded change
	n, err := snappy.DecodedLen(decrypted)
	if err != nil {
		return nil, err
	}
	buf = slices.Grow(buf, n)[:n]
	var decoded []byte
	decoded, err = snappy.Decode(buf, decrypted)
	if err != nil {
		return nil, err
	}
	err = proto.Unmarshal(decoded, ch)
	if err != nil {
		return nil, err
	}
	return ch, nil
}

func (c *changeDataConverter) Marshall(model any) ([]byte, error) {
	ch, ok := model.(*pb.Change)
	if !ok {
		return nil, fmt.Errorf("can't convert the model")
	}
	if c.anonymize {
		ch = anonymize.Change(ch)
	}
	return ch.Marshal()
}
