package debug

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/anonymize"
	"github.com/gogo/protobuf/proto"
)

type changeDataConverter struct {
	anonymize bool
}

func (c *changeDataConverter) Unmarshall(decrypted []byte) (res any, err error) {
	ch := &pb.Change{}
	err = proto.Unmarshal(decrypted, ch)
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
