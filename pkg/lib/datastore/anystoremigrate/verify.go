package anystoremigrate

import (
	"context"
	"fmt"
	"slices"

	anystorev1 "github.com/anyproto/any-store"
	anystore "github.com/anyproto/any-store/v2"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

// verify checks the v2 database against the v1 one it was built from: the same collections, the
// same number of documents in each (a chat messages collection less its dropped stubs), the same
// index definitions (a chat messages collection: the chat repository's, and its storage key), and
// a consistent file (QuickCheck). Index definitions are compared field by field: a dropped Unique
// or Sparse flag changes no count and raises no error.
func verify(ctx context.Context, src anystorev1.DB, dst anystore.DB, stubs map[string]int) error {
	srcNames, err := src.GetCollectionNames(ctx)
	if err != nil {
		return fmt.Errorf("list v1 collections: %w", err)
	}
	dstNames, err := dst.GetCollectionNames(ctx)
	if err != nil {
		return fmt.Errorf("list v2 collections: %w", err)
	}
	slices.Sort(srcNames)
	slices.Sort(dstNames)
	if !slices.Equal(srcNames, dstNames) {
		return fmt.Errorf("collections differ: v1 %v, v2 %v", srcNames, dstNames)
	}
	for _, name := range srcNames {
		if err = verifyCollection(ctx, src, dst, name, stubs[name]); err != nil {
			return fmt.Errorf("collection %q: %w", name, err)
		}
	}
	if err = dst.QuickCheck(ctx); err != nil {
		return fmt.Errorf("quick check: %w", err)
	}
	return nil
}

func verifyCollection(ctx context.Context, src anystorev1.DB, dst anystore.DB, name string, stubs int) error {
	srcColl, err := src.OpenCollection(ctx, name)
	if err != nil {
		return fmt.Errorf("open v1 collection: %w", err)
	}
	defer srcColl.Close()
	dstColl, err := dst.OpenCollection(ctx, name)
	if err != nil {
		return fmt.Errorf("open v2 collection: %w", err)
	}
	defer dstColl.Close()

	srcCount, err := srcColl.Count(ctx)
	if err != nil {
		return fmt.Errorf("count v1 documents: %w", err)
	}
	dstCount, err := dstColl.Count(ctx)
	if err != nil {
		return fmt.Errorf("count v2 documents: %w", err)
	}
	if want := srcCount - stubs; dstCount != want {
		return fmt.Errorf("%d documents, want %d (%d in v1, %d stubs dropped)", dstCount, want, srcCount, stubs)
	}

	var want []anystore.IndexInfo
	if chatmodel.IsMessagesCollection(name) {
		if pk := dstColl.PrimaryKey(); pk != chatmodel.StorageKeyField {
			return fmt.Errorf("primary key %q, want %q", pk, chatmodel.StorageKeyField)
		}
		want = chatmodel.MessagesCollectionIndexes()
	} else {
		for _, idx := range srcColl.GetIndexes() {
			info := idx.Info()
			want = append(want, anystore.IndexInfo{Name: info.Name, Fields: info.Fields, Unique: info.Unique, Sparse: info.Sparse})
		}
	}
	got := make(map[string]anystore.IndexInfo, len(want))
	for _, idx := range dstColl.GetIndexes() {
		info := idx.Info()
		got[info.Name] = info
	}
	if len(got) != len(want) {
		return fmt.Errorf("%d indexes, want %d", len(got), len(want))
	}
	for _, w := range want {
		g, ok := got[w.Name]
		if !ok {
			return fmt.Errorf("index %q is missing", w.Name)
		}
		if !slices.Equal(g.Fields, w.Fields) || g.Unique != w.Unique || g.Sparse != w.Sparse {
			return fmt.Errorf("index %q is %v unique=%v sparse=%v, want %v unique=%v sparse=%v",
				w.Name, g.Fields, g.Unique, g.Sparse, w.Fields, w.Unique, w.Sparse)
		}
	}
	return nil
}
