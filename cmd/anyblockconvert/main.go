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
	"os"
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

	formats, err := anyblockbatch.ScanFormats(files)
	if err != nil {
		return fmt.Errorf("scan property formats: %w", err)
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
	b := newBatch(formats)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	var failed int
	var converted int
	for _, f := range files {
		id, sbType, snap, err := convertFile(inDir, f, b, normalizeIndent)
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

	for _, p := range b.pending {
		if err := writeSnapshot(outDir, p.id, p.sbType, p.snapshot, format); err != nil {
			return fmt.Errorf("write synthesized %s: %w", p.id, err)
		}
	}

	fmt.Printf("\n%d documents converted, %d failed\n", converted, failed)
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
