// Command readcounter-bandsize measures the "band" / concurrency structure of
// real chat change DAGs, to decide whether Option B (the hybrid read counter:
// cheap dominance fast path + bounded DAG fallback on the ambiguous band) is
// cheap enough in practice — i.e. whether "Property M" (forks merge promptly, so
// the band is small) holds on real data.
//
// See docs/superpowers/specs/2026-06-03-read-counter-hybrid-design.md.
//
// Usage:
//
//	go run ./cmd/readcounter-bandsize -db <path-to-changes-db> [-tree id] [-top N] [-bandlimit N]
//
// The DB is any any-store database that holds the any-sync object-tree "changes"
// collection (the per-space CRDT store). The live app DB may be locked — copy it
// first and point -db at the copy.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/util/storeutil"
)

// Doc keys of the object-tree "changes" collection (any-sync objecttree/storage.go).
const (
	collChanges = "changes"
	keyID       = "id"
	keyOrder    = "o" // OrderId (lexid; replica-invariant order)
	keyPrev     = "p" // PrevIds (parents)
	keyTree     = "t" // TreeId
	keyAddSeq   = "q" // AddSeq (device-local; not used by any metric)
)

func main() {
	dbPath := flag.String("db", "", "path to the any-store DB holding the 'changes' collection (copy of a space CRDT store)")
	treeID := flag.String("tree", "", "analyze only this tree id (default: all)")
	top := flag.Int("top", 15, "analyze the N largest trees (0 = all)")
	bandLimit := flag.Int("bandlimit", 60000, "skip the exact O(N^2/64) band for trees larger than this (structural metrics still printed)")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "usage: readcounter-bandsize -db <path-to-changes-db> [-tree id] [-top N] [-bandlimit N]")
		fmt.Fprintln(os.Stderr, "the DB must contain the any-sync object-tree 'changes' collection;")
		fmt.Fprintln(os.Stderr, "copy the running app's space store DB first (the live one may be locked).")
		os.Exit(2)
	}
	if err := run(*dbPath, *treeID, *top, *bandLimit); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(dbPath, treeFilter string, top, bandLimit int) error {
	ctx := context.Background()
	db, err := anystore.Open(ctx, dbPath, nil)
	if err != nil {
		return fmt.Errorf("open db %q: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	names, err := db.GetCollectionNames(ctx)
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	if !contains(names, collChanges) {
		return fmt.Errorf("collection %q not found; collections present: %v (point -db at the object-tree store)", collChanges, names)
	}
	coll, err := db.OpenCollection(ctx, collChanges)
	if err != nil {
		return fmt.Errorf("open collection %q: %w", collChanges, err)
	}

	trees, err := loadTrees(ctx, coll, treeFilter)
	if err != nil {
		return err
	}
	if len(trees) == 0 {
		fmt.Println("no trees found")
		return nil
	}

	type treeChanges struct {
		id string
		n  []ChangeNode
	}
	sorted := make([]treeChanges, 0, len(trees))
	for id, n := range trees {
		sorted = append(sorted, treeChanges{id, n})
	}
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i].n) > len(sorted[j].n) })
	if top > 0 && treeFilter == "" && len(sorted) > top {
		sorted = sorted[:top]
	}

	fmt.Printf("DB %s — %d trees total, analyzing %d\n\n", dbPath, len(trees), len(sorted))
	agg := make([]TreeMetrics, 0, len(sorted))
	for _, t := range sorted {
		m := ComputeMetrics(t.id, t.n, len(t.n) <= bandLimit)
		agg = append(agg, m)
		printMetrics(m, bandLimit)
	}
	printVerdict(agg)
	return nil
}

func loadTrees(ctx context.Context, coll anystore.Collection, treeFilter string) (map[string][]ChangeNode, error) {
	iter, err := coll.Find(nil).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("iter changes: %w", err)
	}
	defer func() { _ = iter.Close() }()

	trees := map[string][]ChangeNode{}
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("read doc: %w", err)
		}
		v := doc.Value()
		tid := v.GetString(keyTree)
		if treeFilter != "" && tid != treeFilter {
			continue
		}
		trees[tid] = append(trees[tid], ChangeNode{
			Id:      v.GetString(keyID),
			OrderId: v.GetString(keyOrder),
			PrevIds: storeutil.StringsFromArrayValue(v, keyPrev),
			AddSeq:  uint64(v.GetInt(keyAddSeq)),
		})
	}
	return trees, iter.Err()
}

func printMetrics(m TreeMetrics, bandLimit int) {
	concPct := 0.0
	if m.N > 0 {
		concPct = 100 * float64(m.ConcurrentPos) / float64(m.N)
	}
	fmt.Printf("tree %s\n", short(m.TreeID))
	fmt.Printf("  N=%d roots=%d heads=%d merges=%d (maxMergeWidth=%d)\n",
		m.N, m.Roots, m.Heads, m.Merges, m.MaxMergeWidth)
	fmt.Printf("  open-width: max=%d  concurrent-positions=%.2f%% (%d/%d)  longest-unmerged-run=%d\n",
		m.MaxOpenWidth, concPct, m.ConcurrentPos, m.N, m.LongestConcurrent)
	fmt.Printf("  fork-span(OrderId gap): p50=%d p95=%d max=%d\n", m.ForkSpanP50, m.ForkSpanP95, m.ForkSpanMax)
	if m.BandComputed {
		fmt.Printf("  band(single-head): max=%d p50=%d p95=%d mean=%.3f  nonzero-heads=%d/%d\n",
			m.BandMax, m.BandP50, m.BandP95, m.BandMean, m.BandNonZero, m.N)
	} else {
		fmt.Printf("  band: SKIPPED (N>%d; raise -bandlimit to force)\n", bandLimit)
	}
	fmt.Println()
}

func printVerdict(agg []TreeMetrics) {
	maxBand, maxWidth, maxRun := 0, 0, 0
	bandComputed := false
	for _, m := range agg {
		if m.BandComputed {
			bandComputed = true
			if m.BandMax > maxBand {
				maxBand = m.BandMax
			}
		}
		if m.MaxOpenWidth > maxWidth {
			maxWidth = m.MaxOpenWidth
		}
		if m.LongestConcurrent > maxRun {
			maxRun = m.LongestConcurrent
		}
	}
	fmt.Println("=== verdict ===")
	fmt.Printf("max open-width across analyzed trees: %d\n", maxWidth)
	fmt.Printf("longest unmerged run (consecutive width>=2): %d\n", maxRun)
	if !bandComputed {
		fmt.Println("band not computed (all trees above -bandlimit); rerun with a higher -bandlimit on a sample")
		return
	}
	fmt.Printf("max single-head band: %d\n\n", maxBand)
	// The bounded fallback does O(band) work only when band>0 (else two index
	// scans). band is always <= the tree size, so Option B is never worse than the
	// old whole-tree cold-start walk; the question is only absolute cost.
	switch {
	case maxBand == 0:
		fmt.Println("No concurrency at all: the dominance fast path is exact everywhere; the fallback never runs.")
	case maxBand <= 200:
		fmt.Println("Property M holds: bands are small; the bounded fallback is a handful of lookups.")
		fmt.Printf("=> Option B is cheap (worst case ~%d lookups vs the old whole-tree cold-start walk).\n", maxBand)
	case maxBand <= 5000:
		fmt.Println("Moderate bands on the most concurrent trees, but still far below tree size.")
		fmt.Println("=> Option B remains far cheaper than the old whole-tree approach; spot-check the widest trees.")
	default:
		fmt.Println("WARNING: very large bands — a few trees sustain near-tree-wide concurrency.")
		fmt.Println("=> the fallback approaches a full-tree walk for those; consider the")
		fmt.Println("   device-key version-vector (option C1) for the long term, with B for legacy.")
	}
}

func short(s string) string {
	if len(s) > 24 {
		return s[:10] + "…" + s[len(s)-10:]
	}
	return s
}

func contains(ss []string, x string) bool {
	for _, s := range ss {
		if s == x {
			return true
		}
	}
	return false
}
