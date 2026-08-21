package debug

import (
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"

	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/anonymize"
)

// changeDataConverter (de)serializes tree changes for export/import. Regular
// smartblocks encode changes as pb.Change; store-backed objects (chats,
// discussions, etc., see smartblock.SmartBlockType.IsStoreBacked) encode
// changes as pb.StoreChange instead. storeBacked picks which wire format to use.
type changeDataConverter struct {
	anonymize   bool
	storeBacked bool
}

func (c *changeDataConverter) Unmarshall(dataType string, decrypted []byte) (res any, err error) {
	if c.storeBacked {
		return sourceimpl.UnmarshalStoreChange(&objecttree.Change{DataType: dataType}, decrypted)
	}
	return sourceimpl.UnmarshalChangeWithDataType(dataType, decrypted)
}

func (c *changeDataConverter) Marshall(model any) (data []byte, dataType string, err error) {
	if c.storeBacked {
		ch, ok := model.(*pb.StoreChange)
		if !ok {
			return nil, "", fmt.Errorf("can't convert the model")
		}
		data, err = ch.Marshal()
		return
	}
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
