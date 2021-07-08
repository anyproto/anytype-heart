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

type DebugTreeStats struct {
	RecordCount      int
	SnapshotCount    int
	ChangeCount      int
	LogCount         int
	NotEmptyLogCount int
	ReadErrorCount   int
}

func (dts DebugTreeStats) String() string {
	return fmt.Sprintf("logs: %d (%d); records: %d; snapshots: %d; changes: %d; errors: %d",
		dts.LogCount,
		dts.NotEmptyLogCount,
		dts.RecordCount,
		dts.SnapshotCount,
		dts.ChangeCount,
		dts.ReadErrorCount,
	)
}

func (dts DebugTreeStats) MlString() string {
	return fmt.Sprintf("Logs:\t%d (%d)\nRecords:\t%d\nSnapshots:\t%d\nChanges:\t%d\nErrors:\t%d\n",
		dts.LogCount,
		dts.NotEmptyLogCount,
		dts.RecordCount,
		dts.SnapshotCount,
		dts.ChangeCount,
		dts.ReadErrorCount,
	)
}

type DebugTree interface {
	core.SmartBlock
	Stats() DebugTreeStats
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
	ch, err := r.getChange(recordID)
	if err != nil {
		return nil, fmt.Errorf("record '%s' %v", recordID, err)
	}
	var pl []byte
	if ch.Change != nil {
		pl, _ = ch.Change.Marshal()
	}
	return &core.SmartblockRecordEnvelope{
		SmartblockRecord: core.SmartblockRecord{
			ID:      ch.Id,
			Payload: pl,
		},
		AccountID: ch.Account,
		LogID:     ch.Device,
	}, nil
}

func (r *debugTree) getChange(id string) (ch *change.Change, err error) {
	for _, f := range r.zr.File {
		if f.Name == id+".json" {
			rd, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rd.Close()
			var ch = &change.Change{}
			if err = json.NewDecoder(rd).Decode(ch); err != nil {
				return nil, err
			}

			return ch, nil
		}
	}
	return nil, fmt.Errorf("not found")
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

func (r *debugTree) Stats() (s DebugTreeStats) {
	logs, _ := r.GetLogs()
	s.LogCount = len(logs)
	for _, l := range logs {
		if l.Head != "" {
			s.NotEmptyLogCount++
		}
	}
	for _, f := range r.zr.File {
		if filepath.Ext(f.Name) == ".json" && f.Name != "block_logs.json" {
			l, err := r.getChange(strings.ReplaceAll(f.Name, ".json", ""))
			if err == nil {
				if l.Snapshot != nil {
					s.SnapshotCount++
				}
				s.RecordCount++
				s.ChangeCount += len(l.Content)
			} else {
				s.ReadErrorCount++
			}
		}
	}
	return
}

func (r *debugTree) Close() (err error) {
	return r.zr.Close()
}
