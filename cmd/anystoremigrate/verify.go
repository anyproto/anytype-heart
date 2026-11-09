package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	anystorev1 "github.com/anyproto/any-store"
	anystore "github.com/anyproto/any-store/v2"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoremigrate"
)

type verifyResult struct {
	Databases    int      `json:"databases"`
	Collections  int      `json:"collections"`
	Documents    int      `json:"documents"`
	Chats        int      `json:"chats"`
	ChatMessages int      `json:"chatMessages"`
	Mismatches   int      `json:"mismatches"`
	Examples     []string `json:"examples,omitempty"`
}

func (v *verifyResult) mismatch(format string, args ...any) {
	v.Mismatches++
	if len(v.Examples) < 20 {
		v.Examples = append(v.Examples, fmt.Sprintf(format, args...))
	}
}

// verifyAll compares every migrated database with the v1 file it kept, document by document: the
// same documents byte for byte, and for a chat the same messages, in the same order-id order, with
// only the storage key added and the stubs gone.
func verifyAll(ctx context.Context, reports []anystoremigrate.Report) (*verifyResult, error) {
	res := &verifyResult{}
	for _, r := range reports {
		if r.Path == "" {
			continue
		}
		if err := verifyDatabase(ctx, r.Path, res); err != nil {
			return nil, fmt.Errorf("%s: %w", r.Path, err)
		}
		res.Databases++
	}
	return res, nil
}

func verifyDatabase(ctx context.Context, path string, res *verifyResult) error {
	backup := path + anystoremigrate.V1BackupSuffix
	if _, err := os.Stat(backup); err != nil {
		return fmt.Errorf("kept v1 file: %w", err)
	}
	src, err := anystorev1.Open(ctx, backup, nil)
	if err != nil {
		return fmt.Errorf("open v1: %w", err)
	}
	defer src.Close()
	dst, err := anystore.Open(ctx, path, nil)
	if err != nil {
		return fmt.Errorf("open v2: %w", err)
	}
	defer dst.Close()

	names, err := src.GetCollectionNames(ctx)
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	for _, name := range names {
		srcColl, err := src.OpenCollection(ctx, name)
		if err != nil {
			return fmt.Errorf("open v1 %s: %w", name, err)
		}
		dstColl, err := dst.OpenCollection(ctx, name)
		if err != nil {
			srcColl.Close()
			return fmt.Errorf("open v2 %s: %w", name, err)
		}
		if chatmodel.IsMessagesCollection(name) {
			err = verifyChat(ctx, srcColl, dstColl, path, name, res)
		} else {
			err = verifyPlain(ctx, srcColl, dstColl, path, name, res)
		}
		srcColl.Close()
		dstColl.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		res.Collections++
	}
	return nil
}

// verifyPlain finds every v1 document in the v2 collection by id: the same bytes. The counts are
// compared too, so nothing extra is in the v2 collection.
func verifyPlain(ctx context.Context, srcColl anystorev1.Collection, dstColl anystore.Collection, path, name string, res *verifyResult) error {
	srcCount, err := srcColl.Count(ctx)
	if err != nil {
		return fmt.Errorf("count v1: %w", err)
	}
	dstCount, err := dstColl.Count(ctx)
	if err != nil {
		return fmt.Errorf("count v2: %w", err)
	}
	if srcCount != dstCount {
		res.mismatch("%s %s: %d documents, want %d", path, name, dstCount, srcCount)
	}
	srcIter, err := srcColl.Find(nil).Iter(ctx)
	if err != nil {
		return fmt.Errorf("iterate v1: %w", err)
	}
	defer srcIter.Close()
	var srcBuf, dstBuf []byte
	for srcIter.Next() {
		srcDoc, err := srcIter.Doc()
		if err != nil {
			return fmt.Errorf("read v1: %w", err)
		}
		srcBuf = srcDoc.Value().MarshalTo(srcBuf[:0])
		// the heart's ids are strings
		id := srcDoc.Value().GetString("id")
		if id == "" {
			res.mismatch("%s %s: a document without a string id: %s", path, name, srcDoc.Value().String())
			continue
		}
		dstDoc, err := dstColl.FindId(ctx, id)
		if err != nil {
			res.mismatch("%s %s: document %s: %v", path, name, id, err)
			continue
		}
		dstBuf = dstDoc.Value().MarshalTo(dstBuf[:0])
		if !bytes.Equal(srcBuf, dstBuf) {
			res.mismatch("%s %s: document %s differs", path, name, id)
		}
		res.Documents++
	}
	return srcIter.Err()
}

type chatMessage struct {
	orderId, id string
	raw         []byte
}

// verifyChat reads the v1 messages by _o.id and the v2 ones by storage key: the same order ids in
// the same sequence, the same messages at each order id, byte for byte once the storage key is
// taken out.
func verifyChat(ctx context.Context, srcColl anystorev1.Collection, dstColl anystore.Collection, path, name string, res *verifyResult) error {
	var want, got []chatMessage
	srcIter, err := srcColl.Find(nil).Sort("_o.id").Iter(ctx)
	if err != nil {
		return fmt.Errorf("iterate v1: %w", err)
	}
	for srcIter.Next() {
		doc, err := srcIter.Doc()
		if err != nil {
			srcIter.Close()
			return fmt.Errorf("read v1: %w", err)
		}
		orderId := doc.Value().GetString("_o", "id")
		if orderId == "" {
			continue // a legacy stub: dropped
		}
		want = append(want, chatMessage{orderId: orderId, id: doc.Value().GetString("id"), raw: doc.Value().MarshalTo(nil)})
	}
	if err = errors.Join(srcIter.Err(), srcIter.Close()); err != nil {
		return fmt.Errorf("iterate v1: %w", err)
	}
	dstIter, err := dstColl.Find(nil).Sort(chatmodel.SortStorageKeyAsc).Iter(ctx)
	if err != nil {
		return fmt.Errorf("iterate v2: %w", err)
	}
	for dstIter.Next() {
		doc, err := dstIter.Doc()
		if err != nil {
			dstIter.Close()
			return fmt.Errorf("read v2: %w", err)
		}
		val := doc.Value()
		if key := val.GetString(chatmodel.StorageKeyField); key != chatmodel.StorageKey(val.GetString("_o", "id"), val.GetString("id")) {
			res.mismatch("%s %s: message %s has key %q", path, name, val.GetString("id"), key)
		}
		val.Del(chatmodel.StorageKeyField)
		got = append(got, chatMessage{orderId: val.GetString("_o", "id"), id: val.GetString("id"), raw: val.MarshalTo(nil)})
	}
	if err = errors.Join(dstIter.Err(), dstIter.Close()); err != nil {
		return fmt.Errorf("iterate v2: %w", err)
	}

	res.Chats++
	res.ChatMessages += len(got)
	if len(want) != len(got) {
		res.mismatch("%s %s: %d messages, want %d", path, name, len(got), len(want))
		return nil
	}
	for i := range want {
		if want[i].orderId != got[i].orderId {
			res.mismatch("%s %s: message %d has order %q, want %q", path, name, i, got[i].orderId, want[i].orderId)
			return nil
		}
	}
	// within one order id (concurrent messages) v1 ordered by id, v2 by the id tail: compare each
	// run of one order id sorted by id
	sortRuns(want)
	sortRuns(got)
	for i := range want {
		if want[i].id != got[i].id {
			res.mismatch("%s %s: message %s at order %q, want %s", path, name, got[i].id, got[i].orderId, want[i].id)
			continue
		}
		if !bytes.Equal(want[i].raw, got[i].raw) {
			res.mismatch("%s %s: message %s differs", path, name, want[i].id)
		}
	}
	return nil
}

// sortRuns sorts each run of messages with one order id by id.
func sortRuns(msgs []chatMessage) {
	for start := 0; start < len(msgs); {
		end := start + 1
		for end < len(msgs) && msgs[end].orderId == msgs[start].orderId {
			end++
		}
		slices.SortFunc(msgs[start:end], func(a, b chatMessage) int { return strings.Compare(a.id, b.id) })
		start = end
	}
}
