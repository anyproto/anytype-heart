package main

import (
	"fmt"
	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: anyblockvalidate <file-or-dir>...")
		os.Exit(2)
	}
	var files, indexes []string
	for _, arg := range os.Args[1:] {
		_ = filepath.Walk(arg, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() || !strings.HasSuffix(p, ".json") {
				return nil
			}
			// the bundle index (§2c) describes the bundle, not an object: it
			// has its own schema and would fail every object-level check
			if filepath.Base(p) == anyblockjson.IndexFileName {
				indexes = append(indexes, p)
				return nil
			}
			files = append(files, p)
			return nil
		})
	}
	fail, warned := 0, 0

	if len(indexes) == 0 {
		warned++
		fmt.Printf("warn    no index.json found\n         without one the space has no name, no entry point and no sidebar (§2c)\n")
	}
	for _, idxPath := range indexes {
		data, err := os.ReadFile(idxPath)
		if err != nil {
			fmt.Printf("READERR %s: %v\n", idxPath, err)
			fail++
		} else if idx, err := anyblockjson.UnmarshalIndex(data); err != nil {
			fmt.Printf("INVALID %s\n         %v\n", idxPath, err)
			fail++
		} else {
			dangling := anyblockbatch.CheckIndexTargets(idx, files)
			if len(dangling) > 0 {
				fmt.Printf("INVALID %s\n%s", idxPath, anyblockbatch.ReportTargets(dangling))
				fail += len(dangling)
			} else {
				entry := idx.EntryPoint()
				if entry == "" {
					entry = "(nothing — no entrypoint declared)"
				}
				home := idx.SpaceHomepage()
				if home == "" {
					home = "(the widgets screen)"
				}
				fmt.Printf("ok      %s\n         install opens %s · space homepage %s\n", idxPath, entry, home)
			}
		}
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf("READERR %s: %v\n", f, err)
			fail++
			continue
		}
		var warnings []anyblockjson.Issue
		err = anyblockjson.ValidateWarn(data, func(i anyblockjson.Issue) {
			warnings = append(warnings, i)
		})
		if err != nil {
			fmt.Printf("INVALID %s\n         %v\n", f, err)
			fail++
			continue
		}
		if len(warnings) > 0 {
			warned++
			fmt.Printf("warn    %s\n", f)
			for _, w := range warnings {
				fmt.Printf("         %v\n", w)
			}
			continue
		}
		fmt.Printf("ok      %s\n", f)
	}
	if shared, serr := anyblockbatch.CheckSharedSelects(files); serr == nil && len(shared) > 0 {
		warned += len(shared)
		fmt.Printf("\nSHARED select properties:\n%s", anyblockbatch.ReportSharedSelects(shared))
	}

	// batch-wide: a property whose format no type declares converts to raw
	// JSON — dates stay strings, selects mint no options, object references
	// are never remapped. Per-file validation cannot see it.
	if formats, ferr := anyblockbatch.ScanFormats(files); ferr == nil {
		if undeclared, uerr := anyblockbatch.CheckPropertyFormats(files, formats); uerr == nil && len(undeclared) > 0 {
			fail += len(undeclared)
			fmt.Printf("\nUNDECLARED property formats (anyblockconvert will refuse these):\n%s",
				anyblockbatch.Report(undeclared))
		}
	}

	fmt.Printf("\n=== %d/%d valid, %d invalid", len(files)-fail, len(files), fail)
	if warned > 0 {
		// warnings do not fail the run: the document imports, part of it is
		// just inert
		fmt.Printf(", %d with warnings", warned)
	}
	fmt.Println(" ===")
	if fail > 0 {
		os.Exit(1)
	}
}
