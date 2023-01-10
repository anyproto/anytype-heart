package debug

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/util/anonymize"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type treeBuilder struct {
	log        *log.Logger
	b          core.SmartBlock
	changes    map[string]struct{}
	zw         *zip.Writer
	s          objectstore.ObjectStore
	anonymized bool
}

func (b *treeBuilder) Build(path string) (filename string, err error) {
	filename = filepath.Join(path, fmt.Sprintf("at.dbg.%s.%s.zip", b.b.ID(), time.Now().Format("20060102.150405.99")))
	f, err := os.Create(filename)
	if err != nil {
		return
	}
	defer f.Close()

	b.zw = zip.NewWriter(f)
	defer b.zw.Close()

	logBuf := bytes.NewBuffer(nil)
	b.log = log.New(io.MultiWriter(logBuf, os.Stderr), "", log.LstdFlags)

	// write logs
	b.log.Printf("dump block_logs...")
	logsWr, err := b.zw.Create("block_logs.json")
	if err != nil {
		b.log.Printf("create file in zip error: %v", err)
		return
	}
	logs, err := b.b.GetLogs()
	if err != nil {
		b.log.Printf("block.GetLogs() error: %v", err)
		return
	}
	enc := json.NewEncoder(logsWr)
	enc.SetIndent("", "\t")
	if err = enc.Encode(logs); err != nil {
		b.log.Printf("can't write json: %v", err)
		return
	}
	b.log.Printf("block_logs.json wrote")

	changes, err := b.getChangesFromLogs(logs)
	if err != nil {
		b.log.Printf("get changes: %v", err)
		return
	}
	// write changes
	st := time.Now()
	b.log.Printf("write changes...")
	err = b.writeChanges(changes)
	if err != nil {
		b.log.Printf("writeChanges: %v", err)
		return
	}
	b.log.Printf("wrote %d changes for a %v", len(b.changes), time.Since(st))

	b.log.Printf("write localstore data...")
	data, err := b.s.GetByIDs(b.b.ID())
	if err != nil {
		b.log.Printf("can't fetch localstore info: %v", err)
	} else {
		if len(data) > 0 {
			data[0].Details = anonymize.Struct(data[0].Details)
			data[0].Snippet = anonymize.Text(data[0].Snippet)
			for i, r := range data[0].Relations {
				data[0].Relations[i] = anonymize.Relation(r)
			}
			osData := pbtypes.Sprint(data[0])
			lsWr, er := b.zw.Create("localstore.json")
			if er != nil {
				b.log.Printf("create file in zip error: %v", er)
			} else {
				if _, err := lsWr.Write([]byte(osData)); err != nil {
					b.log.Printf("localstore.json write error: %v", err)
				} else {
					b.log.Printf("localstore.json wrote")
				}
			}
		} else {
			b.log.Printf("not data in objectstore")
		}
	}

	logW, err := b.zw.Create("creation.log")
	if err != nil {
		return
	}
	io.Copy(logW, logBuf)
	return
}

func (b *treeBuilder) getChangesFromLogs(logs []core.SmartblockLog) (changes []*change.Change, err error) {
	b.changes = make(map[string]struct{})
	var q1, buf []string
	for _, l := range logs {
		if l.Head != "" {
			q1 = append(q1, l.Head)
		}
	}

	for len(q1) > 0 {
		buf = buf[:0]
		for _, id := range q1 {
			ch, nextIDs := b.getChange(id)
			changes = append(changes, ch)
			buf = append(buf, nextIDs...)
		}
		q1, buf = buf, q1
	}
	return
}

func (b *treeBuilder) getChange(id string) (ch *change.Change, nextIds []string) {
	if _, ok := b.changes[id]; ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	rec, err := b.b.GetRecord(ctx, id)
	if err != nil {
		b.log.Printf("can't get record: %v: %v", id, err)
		return
	}
	chp := new(pb.Change)
	if err = rec.Unmarshal(chp); err != nil {
		return
	}
	ch = &change.Change{
		Id:      id,
		Account: rec.AccountID,
		Device:  rec.LogID,
	}
	if b.anonymized {
		ch.Change = anonymize.Change(chp)
	} else {
		ch.Change = chp
	}
	b.changes[id] = struct{}{}
	return ch, chp.PreviousIds
}

func (b *treeBuilder) writeChanges(changes []*change.Change) error {
	for _, ch := range changes {
		st := time.Now()
		chw, err := b.zw.CreateHeader(&zip.FileHeader{
			Name:     ch.Id + ".json",
			Method:   zip.Deflate,
			Modified: time.Unix(ch.Timestamp, 0),
		})

		if err != nil {
			b.log.Printf("create file in zip error: %v", err)
			return err
		}
		enc := json.NewEncoder(chw)
		enc.SetIndent("", "\t")
		if err = enc.Encode(ch); err != nil {
			b.log.Printf("can't write json: %v", err)
			return err
		}
		b.changes[ch.Id] = struct{}{}
		b.log.Printf("%v wrote for a %v", ch.Id, time.Since(st))
	}
	return nil
}
