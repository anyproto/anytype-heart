// anyblockconvert converts a directory tree of AnyBlock JSON documents
// (pkg/lib/anyblockjson) into a directory of old-format pb snapshots, laid
// out the way the bundled use-case archives are (util/builtinobjects/data/
// *.zip): one .pb file per object, under objects/types/relations/
// relationsOptions/templates subdirectories. The result can be fed straight
// to the existing pb importer (core/block/import/pb) or zipped into a
// builtinobjects archive.
//
// pkg/lib/anyblockjson.Unmarshal only ever sees one document at a time and
// deliberately leaves cross-document concerns to the caller (SPEC.md calls
// this "the import wiring's job"): resolving a custom property's format,
// and minting the Relation/RelationOption objects a custom property or
// select/multiSelect option needs to exist as (the pb importer relinks ids
// across a batch, but — unlike CSV/Notion import — does not synthesize
// missing relation or option objects on its own). This tool is that wiring,
// built the same way core/block/import/csv and core/block/import/notion
// build their Relation/RelationOption snapshots.
//
// Every document's own "id" (or a deterministic fallback derived from its
// file path, when omitted) passes through untouched: the pb importer already
// relinks references by matching literal id strings across the whole batch
// (core/block/import/common.UpdateLinksToObjects /
// UpdateObjectIDsInRelations), so there's nothing for this tool to rewrite
// there.
//
// Usage:
//
//	go run ./cmd/anyblockconvert -in ~/usecase2/anyblock/01-company-wiki -out ./out
//
// Pass -format json to write jsonpb text (.json) instead of raw proto bytes
// (.pb) — human-readable, and still accepted by core/block/import/pb.
package main

import (
	"flag"
	"fmt"
	"github.com/anyproto/anytype-heart/cmd/internal/anyblockbatch"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"os"
	"path/filepath"
)

func main() {
	var (
		inDir           = flag.String("in", "", "input directory of AnyBlock JSON documents (searched recursively)")
		outDir          = flag.String("out", "", "output directory for pb snapshots")
		normalizeIndent = flag.Bool("normalize-indent", false, "clamp over-deep block indents instead of rejecting them (SPEC.md §4)")
		lenient         = flag.Bool("lenient", false, "downgrade undeclared-property errors to warnings (SPEC.md §3: such values pass through as raw JSON)")
		format          = flag.String("format", "pb", "output snapshot format: \"pb\" (raw proto, .pb) or \"json\" (jsonpb text, .json; human-readable, still importable via core/block/import/pb)")
	)
	flag.Parse()

	if *inDir == "" || *outDir == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\nboth -in and -out are required")
		os.Exit(2)
	}
	var outFormat outputFormat
	switch *format {
	case "pb":
		outFormat = formatPb
	case "json":
		outFormat = formatJSON
	default:
		fmt.Fprintf(os.Stderr, "invalid -format %q: must be \"pb\" or \"json\"\n", *format)
		os.Exit(2)
	}
	if err := run(*inDir, *outDir, *normalizeIndent, *lenient, outFormat); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(inDir, outDir string, normalizeIndent, lenient bool, format outputFormat) error {
	files, err := anyblockbatch.DiscoverJSONFiles(inDir)
	if err != nil {
		return fmt.Errorf("discover input files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no .json files found under %s", inDir)
	}

	// types declare what objects reference, so convert them first
	files, err = anyblockbatch.OrderTypesFirst(files)
	if err != nil {
		return fmt.Errorf("order input files: %w", err)
	}

	// the `_` namespace belongs to the platform (§1), and the reserved
	// index.json listings live in it. Checked before anything is written: an
	// id this tool would refuse must not first appear as a converted snapshot
	// on disk.
	reservedIds, err := anyblockbatch.CheckBundleIds(files)
	if err != nil {
		return fmt.Errorf("check bundle ids: %w", err)
	}
	if len(reservedIds) > 0 {
		return fmt.Errorf("%d object%s claiming a reserved id:\n%s",
			len(reservedIds), plural2(len(reservedIds)), anyblockbatch.ReportTargets(reservedIds))
	}

	formats, err := anyblockbatch.ScanFormats(files)
	if err != nil {
		return fmt.Errorf("scan property formats: %w", err)
	}

	// the property dictionary (§2f) is a declaration source beside the type
	// documents: an author declares a property once, bundle-wide, without
	// writing a relation document at all. Its entries join the format table
	// (dictionary winning a stated conflict) and are pre-minted below, so a
	// dictionary-declared property exists in the archive whether or not any
	// type happens to list it.
	var dictDefs []anyblockjson.PropertyDefinition
	if dictPath, ok := anyblockbatch.PropertiesPath(inDir); ok {
		dictFormats, defs, dictErr := anyblockbatch.DictionaryFormats(dictPath)
		if dictErr != nil {
			return fmt.Errorf("read property dictionary: %w", dictErr)
		}
		dictDefs = defs
		formats = anyblockbatch.MergeDictionaryFormats(formats, dictFormats, func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
		})
	}

	if shared, serr := anyblockbatch.CheckSharedSelects(files); serr == nil && len(shared) > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d select propert%s shared across types:\n%s",
			len(shared), plural(len(shared)), anyblockbatch.ReportSharedSelects(shared))
	}

	// a property whose format nothing declares is decoded as raw JSON: dates
	// stay strings, selects mint no options, and object references are never
	// remapped, so cross-object links break silently after import
	undeclared, err := anyblockbatch.CheckPropertyFormats(files, formats)
	if err != nil {
		return fmt.Errorf("check property formats: %w", err)
	}
	if len(undeclared) > 0 {
		if !lenient {
			return fmt.Errorf("%d propert%s with no declared format:\n%spass -lenient to convert anyway",
				len(undeclared), plural(len(undeclared)), anyblockbatch.Report(undeclared))
		}
		fmt.Fprintf(os.Stderr, "warning: %d propert%s with no declared format:\n%s",
			len(undeclared), plural(len(undeclared)), anyblockbatch.Report(undeclared))
	}
	typeIds, err := anyblockbatch.TypeIds(files)
	if err != nil {
		return fmt.Errorf("index type ids: %w", err)
	}
	badTargets, err := anyblockbatch.CheckTargetTypes(files, typeIds)
	if err != nil {
		return fmt.Errorf("check target types: %w", err)
	}
	if len(badTargets) > 0 {
		return fmt.Errorf("%d unresolvable object_types target%s:\n%s",
			len(badTargets), map[bool]string{true: "", false: "s"}[len(badTargets) == 1],
			anyblockbatch.ReportTargets(badTargets))
	}
	// a template whose target type cannot be wired imports as an object no type
	// lists — valid, converted, and unreachable
	badTemplates, err := anyblockbatch.CheckTemplateTargets(files, typeIds)
	if err != nil {
		return fmt.Errorf("check template targets: %w", err)
	}
	if len(badTemplates) > 0 {
		return fmt.Errorf("%d template%s with no wirable target type:\n%s",
			len(badTemplates), map[bool]string{true: "", false: "s"}[len(badTemplates) == 1],
			anyblockbatch.ReportTemplateTargets(badTemplates))
	}
	// a fileObject's real bytes are found by its "source" property
	// (SPEC.md §3) at import time — catch a bundle pointing at a file that
	// was never placed under files/ now, not as a silently blank icon later
	danglingSources, err := checkFileSources(inDir, files)
	if err != nil {
		return fmt.Errorf("check file sources: %w", err)
	}
	if len(danglingSources) > 0 {
		return fmt.Errorf("%d dangling file source%s:\n%s",
			len(danglingSources), plural2(len(danglingSources)), reportDanglingSources(danglingSources))
	}

	b := newBatch(formats, typeIds)

	// dictionary-declared properties exist up front, with the FULL declared
	// shape — description, include_time, max_count, readonly, default_value
	// all reach mintRelation (§2e: a member the file admits is never shed at
	// the seam). Entry order is the dictionary's canonical sorted order, so
	// minted ids are stable across runs like everything else in the batch.
	for _, def := range dictDefs {
		b.PropertyId(def)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	// real binary assets (SPEC.md §2c, §3: iconImage, fileObject "source")
	// live in files/ alongside the JSON documents; DiscoverJSONFiles only
	// ever sees the *.json ones, so nothing else copies these into outDir
	copiedFiles, err := copyBundleFiles(inDir, outDir)
	if err != nil {
		return fmt.Errorf("copy bundle files: %w", err)
	}
	if copiedFiles > 0 {
		fmt.Printf("copied %d file(s) into %s\n", copiedFiles, filepath.Join(outDir, "files"))
	}

	var failed int
	var converted int
	var warned int
	for _, f := range files {
		id, sbType, snap, err := convertFile(inDir, f, b, normalizeIndent, func(is anyblockjson.Issue) {
			warned++
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", f, is)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", f, err)
			failed++
			continue
		}
		if err := writeSnapshot(outDir, id, sbType, snap, format); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: write: %v\n", f, err)
			failed++
			continue
		}
		converted++
	}

	// the bundle index (§2c) becomes two outputs: the profile file, which the
	// installer reads for the space's homepage, and a Widget snapshot, which is
	// how the sidebar reaches a space installed as an experience (see
	// widgets.go). Written after the snapshots so a failed conversion does not
	// leave either pointing at nothing.
	if idxPath, ok := anyblockbatch.IndexPath(inDir); ok {
		data, readErr := os.ReadFile(idxPath)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", idxPath, readErr)
		}
		idx, idxErr := anyblockjson.UnmarshalIndex(data)
		if idxErr != nil {
			return fmt.Errorf("%s: %w", anyblockjson.IndexFileName, idxErr)
		}
		if dangling := anyblockbatch.CheckIndexTargets(idx, files); len(dangling) > 0 {
			return fmt.Errorf("%d unresolvable reference%s in %s:\n%s",
				len(dangling), plural2(len(dangling)), anyblockjson.IndexFileName,
				anyblockbatch.ReportTargets(dangling))
		}
		names, nameErr := anyblockbatch.ObjectNames(files)
		if nameErr != nil {
			return fmt.Errorf("index object names: %w", nameErr)
		}
		if err := writeProfile(outDir, idx, names); err != nil {
			return fmt.Errorf("write profile: %w", err)
		}
		// the Widget snapshot carries the id "widgets"; an object claiming the
		// same one would share both that id — and so the importer's relinking
		// entry for it — and the output file with the sidebar. That id is also
		// the wire spelling of the reserved `_widgets` homepage, so
		// CheckBundleIds has already refused it above, for its own reason and
		// before anything was written. This is the backstop for the case where
		// the two stop being the same string.
		if _, taken := names[widgetsObjectId]; taken {
			return fmt.Errorf("an object in the bundle has id %q, which is reserved for the sidebar snapshot (SPEC.md §2c) — rename it", widgetsObjectId)
		}
		if err := writeWidgets(outDir, idx, format); err != nil {
			return fmt.Errorf("write widgets: %w", err)
		}
		entry := idx.EffectiveEntryPoint()
		if entry == "" {
			entry = "(nothing — no widget names an object)"
		}
		home := idx.SpaceHomepage()
		if home == "" {
			home = "(the widgets screen)"
		}
		fmt.Printf("profile written: space %q, homepage %s\n", idx.Name, home)
		fmt.Printf("widgets written: %d sidebar widget(s), in index.json order\n", len(idx.Widgets))
		// TEMPORARY: on the built-in-archive path inject() opens
		// widgets[0].targetObjectId, because pb.Profile has no entry-point
		// field. (On the experience path nothing opens once at all — see §2c —
		// so there the entrypoint only matters through the homepage fallback.)
		if declared := idx.EntryPoint(); declared != "" && declared != entry {
			fmt.Fprintf(os.Stderr, "warning: entrypoint %q is not the first widget, so on the built-in-archive path it is not what opens — inject uses widgets[0] (%s)\n",
				declared, entry)
		}
	} else {
		fmt.Fprintf(os.Stderr, "warning: no %s — the space gets no name, no entry point and no sidebar (SPEC.md \u00a72c)\n",
			anyblockjson.IndexFileName)
	}

	for _, p := range b.pending {
		if err := writeSnapshot(outDir, p.id, p.sbType, p.snapshot, format); err != nil {
			return fmt.Errorf("write synthesized %s: %w", p.id, err)
		}
	}

	fmt.Printf("\n%d documents converted, %d failed", converted, failed)
	if warned > 0 {
		fmt.Printf(", %d warning(s)", warned)
	}
	fmt.Println()
	fmt.Printf("synthesized %d relations, %d relation options\n", b.relationCount(), b.optionCount())
	fmt.Println("output:", outDir)
	if failed > 0 {
		return fmt.Errorf("%d documents failed to convert", failed)
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func plural2(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
