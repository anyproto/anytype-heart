// relsweeptmp — throwaway corpus sweep for the §2d change. NOT committed.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/snapshotdiff"
)

func main() {
	root := os.Args[1]
	var files []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".pb") {
			files = append(files, path)
		}
		return nil
	})
	fmt.Println("pb files:", len(files))
	var total, relations, exportErr, validateErr, importErr, unstable, diffs, warnDocs int
	diffCounts := map[string]int{}
	seq := func() func() string {
		n := 0
		return func() string { n++; return fmt.Sprintf("gen%06d", n) }
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var sw pb.SnapshotWithType
		if err := proto.Unmarshal(raw, &sw); err != nil || sw.Snapshot == nil || sw.Snapshot.Data == nil {
			continue
		}
		total++
		if sw.SbType.String() == "STRelation" {
			relations++
		}
		opts := anyblockjson.Options{}
		var warns []anyblockjson.Issue
		opts.OnWarning = func(i anyblockjson.Issue) { warns = append(warns, i) }
		data, err := anyblockjson.Marshal(sw.SbType, sw.Snapshot.Data, opts)
		if err != nil {
			exportErr++
			fmt.Println("EXPORT ERR:", f, err)
			continue
		}
		if len(warns) > 0 {
			warnDocs++
			for _, w := range warns {
				m := w.Message
				if len(m) > 60 {
					m = m[:60]
				}
				fmt.Println("WARNCLASS:", w.Path, "|", m)
			}
		}
		if err := anyblockjson.Validate(data); err != nil {
			validateErr++
			fmt.Println("VALIDATE ERR (I1!):", f)
			es := err.Error()
			if len(es) > 400 {
				es = es[:400]
			}
			fmt.Println("  ", es)
			continue
		}
		iopts := anyblockjson.Options{GenerateId: seq()}
		sbType2, snap2, err := anyblockjson.Unmarshal(data, iopts)
		if err != nil {
			importErr++
			fmt.Println("IMPORT ERR:", f, err)
			continue
		}
		data2, err := anyblockjson.Marshal(sbType2, snap2, anyblockjson.Options{})
		if err != nil {
			exportErr++
			fmt.Println("REEXPORT ERR:", f, err)
			continue
		}
		// SS11 guarantee 3 allows two documented one-time losses (minted ids,
		// the attribution pair), so stability is measured from the second
		// generation: Export(Import(Export(Import(Export(S))))) == gen2.
		sbType3, snap3, err := anyblockjson.Unmarshal(data2, anyblockjson.Options{GenerateId: seq()})
		if err != nil {
			importErr++
			fmt.Println("REIMPORT ERR:", f, err)
			continue
		}
		data3, err := anyblockjson.Marshal(sbType3, snap3, anyblockjson.Options{})
		if err != nil {
			exportErr++
			continue
		}
		if !bytes.Equal(data2, data3) {
			unstable++
			if unstable <= 5 {
				fmt.Println("UNSTABLE:", f)
			}
		}
		if sw.SbType.String() == "STRelation" {
			if found := snapshotdiff.Compare(sw.Snapshot.Data, snap2, sw.SbType, anyblockjson.Options{}); len(found) > 0 {
				diffs++
				for _, d := range found {
					head := d
					if i := strings.Index(head, ":"); i > 0 {
						head = head[:i]
					}
					diffCounts[head]++
				}
				if diffs <= 5 {
					fmt.Println("DIFF:", f, found)
				}
			}
		}
	}
	fmt.Printf("total=%d relations=%d exportErr=%d validateErr=%d importErr=%d unstable=%d relDiffObjects=%d warnDocs=%d\n",
		total, relations, exportErr, validateErr, importErr, unstable, diffs, warnDocs)
	for k, v := range diffCounts {
		fmt.Printf("  diff %q x%d\n", k, v)
	}
}
