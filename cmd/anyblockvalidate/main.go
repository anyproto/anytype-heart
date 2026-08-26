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
	var files, indexes, dictionaries []string
	for _, arg := range os.Args[1:] {
		_ = filepath.Walk(arg, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() || !strings.HasSuffix(p, ".json") {
				return nil
			}
			// the bundle index (§2c) and the property dictionary (§2f)
			// describe the bundle, not an object: each has its own schema
			// and would fail every object-level check
			if filepath.Base(p) == anyblockjson.IndexFileName {
				indexes = append(indexes, p)
				return nil
			}
			if filepath.Base(p) == anyblockjson.PropertiesFileName {
				dictionaries = append(dictionaries, p)
				return nil
			}
			files = append(files, p)
			return nil
		})
	}
	fail, warned := 0, 0

	// the `_` namespace is the platform's (§1). anyblockconvert refuses a
	// bundle that mints an id in it, so this tool has to see it too — a bundle
	// this one blesses and that one rejects is the worst of both.
	if reserved, err := anyblockbatch.CheckBundleIds(files); err != nil {
		fmt.Printf("READERR %v\n", err)
		fail++
	} else if len(reserved) > 0 {
		fmt.Printf("INVALID %d object(s) claiming a reserved id\n%s", len(reserved), anyblockbatch.ReportTargets(reserved))
		fail += len(reserved)
	}

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
			// the manifest's blob bindings are index references too (§2c,
			// v0.47), and theirs is the other silent failure: a file
			// document whose bytes the entry promises and the archive does
			// not carry
			dangling = append(dangling, anyblockbatch.CheckManifestFiles(idx, filepath.Dir(idxPath), files)...)
			if len(dangling) > 0 {
				fmt.Printf("INVALID %s\n%s", idxPath, anyblockbatch.ReportTargets(dangling))
				fail += len(dangling)
			} else {
				declared, effective := idx.EntryPoint(), idx.EffectiveEntryPoint()
				home := idx.SpaceHomepage()
				if home == "" {
					home = "(the widgets screen)"
				}
				shown := effective
				if shown == "" {
					shown = "(nothing — no widget names an object)"
				}
				fmt.Printf("ok      %s\n         space homepage %s · %d sidebar widget(s)\n",
					idxPath, home, len(idx.Widgets))
				// TEMPORARY: pb.Profile has no entry-point field, so the
				// built-in-archive path (inject) opens widgets[0]. A declared
				// entrypoint that is not the first widget is silently not
				// honoured there. On the experience path — what a bundle
				// actually takes — nothing opens once at all, so the entrypoint
				// only reaches the space as the homepage fallback (§2c).
				if declared != "" && declared != effective {
					warned++
					fmt.Printf("         warn: entrypoint %q is not the first widget, so on the built-in-archive path\n"+
						"               it is NOT what opens — inject uses widgets[0] (%s).\n"+
						"               List the entrypoint first in widgets.\n", declared, effective)
				}
			}
		}
	}
	for _, dictPath := range dictionaries {
		data, err := os.ReadFile(dictPath)
		if err != nil {
			fmt.Printf("READERR %s: %v\n", dictPath, err)
			fail++
			continue
		}
		// the codec TOLERATES an installed key its bundled table cannot name
		// (a newer app's bundled property), but it now SAYS so through the
		// same warn channel object documents have — this tool used to carry
		// its own copy of that check, which meant the authoring surface knew
		// something the format itself did not report.
		var dictWarnings []anyblockjson.Issue
		dict, err := anyblockjson.UnmarshalPropertyDictionaryWarn(data, func(i anyblockjson.Issue) {
			dictWarnings = append(dictWarnings, i)
		})
		if err != nil {
			fmt.Printf("INVALID %s\n         %v\n", dictPath, err)
			fail++
			continue
		}
		fmt.Printf("ok      %s\n         %d installed key(s), %d defined propert%s\n",
			dictPath, len(dict.Installed), len(dict.Properties),
			map[bool]string{true: "y", false: "ies"}[len(dict.Properties) == 1])
		if len(dictWarnings) > 0 {
			warned++
			for _, w := range dictWarnings {
				fmt.Printf("         warn: %s\n", w.String())
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
		// the dictionary is the OTHER home a format can be declared in
		// (§2f), and anyblockconvert merges it before running this exact
		// check. Without the same merge this tool refuses a bundle the
		// converter accepts — and its repair text then sends the author to
		// the type home, undoing the dictionary they had written.
		for _, dictPath := range dictionaries {
			dictFormats, _, derr := anyblockbatch.DictionaryFormats(dictPath)
			if derr != nil {
				continue // the per-file pass above already reported it
			}
			formats = anyblockbatch.MergeDictionaryFormats(formats, dictFormats,
				func(format string, args ...any) {
					warned++
					fmt.Printf("warn    "+format+"\n", args...)
				})
		}
		if undeclared, uerr := anyblockbatch.CheckPropertyFormats(files, formats); uerr == nil && len(undeclared) > 0 {
			fail += len(undeclared)
			fmt.Printf("\nUNDECLARED property formats (anyblockconvert will refuse these):\n%s",
				anyblockbatch.Report(undeclared))
		}

		// batch-wide for the same reason: a view naming a property nothing
		// declares is a filter that matches nothing, a sort that orders
		// nothing, a column that stays empty — and it imports in silence. The
		// CODEC cannot raise it, because a custom property whose stored key is
		// already a legal spelling binds no legend entry, so inside one
		// document a typo and a verbatim custom key look the same. Only here,
		// holding every declaration in the bundle, are they distinguishable.
		declared := map[string]bool{}
		for key := range formats {
			declared[key] = true
		}
		if bad, verr := anyblockbatch.CheckViewProperties(files, declared); verr == nil && len(bad) > 0 {
			fail += len(bad)
			fmt.Printf("\nVIEW slots naming a property nothing declares:\n%s",
				anyblockbatch.ReportViewProperties(bad))
		}
	}

	// every document this run judged, not just the object ones: `files`
	// excludes index.json and properties.json while `fail` counts their
	// failures alongside the batch-wide findings, so subtracting one from
	// the other printed counts that never happened — "-1/0 valid" for a
	// directory holding a single bad dictionary.
	judged := len(files) + len(indexes) + len(dictionaries)
	valid := judged - fail
	if valid < 0 {
		// more findings than documents: a batch-wide check can report
		// several against one file. Say what is true — nothing passed.
		valid = 0
	}
	fmt.Printf("\n=== %d/%d valid, %d invalid", valid, judged, fail)
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
