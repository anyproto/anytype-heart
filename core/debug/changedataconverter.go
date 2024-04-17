package debug

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/anonymize"
)

type changeDataConverter struct {
	anonymize bool
}

func (c *changeDataConverter) Unmarshall(dataType string, decrypted []byte) (res any, err error) {
	return source.UnmarshalChangeWithDataType(dataType, decrypted)
}

func (c *changeDataConverter) Marshall(model any) (data []byte, dataType string, err error) {
	ch, ok := model.(*pb.Change)
	if !ok {
		return nil, "", fmt.Errorf("can't convert the model")
	}
	if c.anonymize {
		ch = anonymize.Change(ch)
	}
	data, err = ch.Marshal()
	return
}
