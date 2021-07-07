package debugtree

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/gogo/protobuf/proto"
)

var ErrNotImplemented = errors.New("not implemented for debug tree")

type DebugTree interface {
	core.SmartBlock
	Close() error
}

// Open expects debug tree zip file
// return DebugTree that implements core.SmartBlock
func Open(filename string) (DebugTree, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	fn := filepath.Base(filename)
	id := ""
	if strings.Count(fn, ".") >= 3 {
		id = strings.Split(fn, ".")[2]
	}
	return &debugTree{
		id: id,
		zr: zr,
	}, nil
}

type debugTree struct {
	id string
	zr *zip.ReadCloser
}

func (r *debugTree) ID() string {
	return r.id
}

func (r *debugTree) Type() smartblock.SmartBlockType {
	if r.id != "" {
		st, err := smartblock.SmartBlockTypeFromID(r.id)
		if err == nil {
			return st
		}
	}
	return smartblock.SmartBlockTypePage
}

func (r *debugTree) Creator() (string, error) {
	return "", nil
}

func (r *debugTree) GetLogs() ([]core.SmartblockLog, error) {
	for _, f := range r.zr.File {
		if f.Name == "block_logs.json" {
			rd, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rd.Close()
			var logs []core.SmartblockLog
			if err = json.NewDecoder(rd).Decode(&logs); err != nil {
				return nil, err
			}
			return logs, nil
		}
	}
	return nil, fmt.Errorf("block logs file not found")
}

func (r *debugTree) GetRecord(ctx context.Context, recordID string) (*core.SmartblockRecordEnvelope, error) {
	for _, f := range r.zr.File {
		if f.Name == recordID+".json" {
			rd, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rd.Close()
			var ch = &change.Change{}
			if err = json.NewDecoder(rd).Decode(ch); err != nil {
				return nil, err
			}
			var pl []byte
			if ch.Change != nil {
				pl, _ = ch.Change.Marshal()
			}
			rec := &core.SmartblockRecordEnvelope{
				SmartblockRecord: core.SmartblockRecord{
					ID:      ch.Id,
					Payload: pl,
				},
				AccountID: ch.Account,
				LogID:     ch.Device,
			}
			return rec, nil
		}
	}
	return nil, fmt.Errorf("record '%s' file not found", recordID)
}

func (*debugTree) PushRecord(payload proto.Marshaler) (id string, err error) {
	return "", ErrNotImplemented
}

func (*debugTree) SubscribeForRecords(ch chan core.SmartblockRecordEnvelope) (cancel func(), err error) {
	return nil, ErrNotImplemented
}

func (*debugTree) SubscribeClientEvents(event chan<- proto.Message) (cancelFunc func(), err error) {
	return nil, ErrNotImplemented
}

func (*debugTree) PublishClientEvent(event proto.Message) error {
	return ErrNotImplemented
}

func (r *debugTree) Close() (err error) {
	return r.zr.Close()
}
