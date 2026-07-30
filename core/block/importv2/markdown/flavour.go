package markdown

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Flavour is the per-run markdown dialect profile (design doc §11.4): the
// converter and engine stay flavour-blind, profiles transform page-local data
// through the hooks below and never see the sink — emission order stays
// converter-owned. Selected once per run: request override first, listing
// detection otherwise. New flavours register a value here plus fixtures; no
// runtime plugin machinery.
//
// Step 1 (seam): every profile reproduces today's behavior exactly — hooks
// are nil and the toggles match the current unconditional code paths. Real
// divergence (Notion field-blocks, Collection store, Obsidian extenders)
// lands with the review-delta fixes.
type Flavour struct {
	Name string

	// Anymark lists extra goldmark extenders enabled for this profile. The
	// base set (tables, strikethrough, toggles, wiki-links) stays global —
	// cheap and unambiguous syntax is not worth gating.
	Anymark []goldmark.Extender

	// ExtractMetadata extracts flavour-specific page metadata beyond the
	// shared YAML front-matter pass, mutating page in place (e.g. Notion's
	// first-paragraph `Key: value` property lines). Runs after H1 title
	// extraction, before reference rewriting. nil = YAML only.
	ExtractMetadata func(c *Converter, page *pageContext)

	// ResolveTarget is the flavour fallback tried after the generic link
	// target chain (exact path → unescape → unique basename) misses. It
	// receives the chain's normalized, percent-unescaped form of the
	// target, not the raw link text.
	ResolveTarget func(c *Converter, target string) (entryName string, ok bool)

	// CSVCollections applies Notion's `Db.csv` ↔ `Db/` convention
	// uniformly: csv files claim collections of their sibling directory's
	// pages, count as link/mention targets, and appear in directory trees.
	CSVCollections bool
	// DirectoryPagesDefault turns directory pages on for this profile in
	// addition to the request param. A plain bool request cannot express
	// explicit-off, so a profile default can only enable — the request
	// param needs a tri-state before any profile ships true (§11.4 open
	// question). Schemas force-disable regardless.
	DirectoryPagesDefault bool
	// CollectionByName additionally treats a front-matter property *named*
	// "Collection" as the collection store. The unambiguous `_collection`
	// key is honored under every profile; the display-name match is too
	// loose to run globally.
	CollectionByName bool
	// SuggestTypes types csv-collection member pages by the collection's
	// title (§11.5, name-only evidence — csv rows are never parsed).
	SuggestTypes bool
	// FolderContainers treats each folder of markdown pages as a schema-plan
	// container (docs/ImportV2LLM.md §3): a front-matter sweep unions the
	// folder's property schema and the planner may type its pages and remap
	// their properties. Obsidian only — vault folders are usually
	// homogeneous; arbitrary markdown trees are not.
	FolderContainers bool
}

// pageContext is the page-local data a metadata hook may inspect and mutate.
type pageContext struct {
	Name   string
	Blocks []*model.Block
}

// Profile names — the wire/config vocabulary for forcing a flavour.
const (
	FlavourGeneric       = "generic"
	FlavourNotionExport  = "notion-export"
	FlavourObsidian      = "obsidian"
	FlavourAnytypeExport = "anytype-export"
)

var flavours = map[string]Flavour{
	FlavourGeneric:  {Name: FlavourGeneric, CSVCollections: true},
	FlavourObsidian: {Name: FlavourObsidian, CSVCollections: true, FolderContainers: true},
	FlavourNotionExport: {
		Name:            FlavourNotionExport,
		ExtractMetadata: notionExtractMetadata,
		ResolveTarget:   notionResolveTarget,
		CSVCollections:  true,
		SuggestTypes:    true,
	},
	FlavourAnytypeExport: {
		Name:             FlavourAnytypeExport,
		CSVCollections:   true,
		CollectionByName: true,
	},
}

// flavourEnables names what each profile's hooks actually switch on, for
// the observability issue.
var flavourEnables = map[string]string{
	FlavourNotionExport:  "property lines, id-based link resolution and collection type suggestion enabled",
	FlavourAnytypeExport: "Collection front matter honored",
}

// resolveFlavour fixes the run's profile: an explicit request name wins,
// otherwise detection over the pass-1 listing. Called once, after the
// listing walk and schema load.
func (c *Converter) resolveFlavour() error {
	if c.params.Flavour != "" {
		flavour, ok := flavours[c.params.Flavour]
		if !ok {
			return importv2.Fatal(importv2.IssueSourceInvalid, fmt.Errorf("unknown markdown flavour %q", c.params.Flavour))
		}
		c.flavour, c.flavourForced = flavour, true
		return nil
	}
	c.flavour = c.detectFlavour()
	return nil
}

// detectFlavour picks the profile from listing signals, strongest first:
// x-app schemas → anytype-export, an .obsidian config dir → obsidian, Notion
// export naming (id-suffixed files or csv↔dir pairs) → notion-export, else
// generic. Signals with false-positive risk gate behavior only through their
// profile, so a wrong guess on a mixed source degrades to generic handling.
func (c *Converter) detectFlavour() Flavour {
	if c.schemas != nil {
		return flavours[FlavourAnytypeExport]
	}
	if c.sawObsidianDir {
		return flavours[FlavourObsidian]
	}
	if c.looksLikeNotionExport() {
		return flavours[FlavourNotionExport]
	}
	return flavours[FlavourGeneric]
}

// notionIdSuffix matches Notion's export naming: "<title> <32-hex-id>.md".
var notionIdSuffix = regexp.MustCompile(` [0-9a-f]{32}\.md$`)

// looksLikeNotionExport fires on ≥ max(2, 20% of pages) id-suffixed file
// names, or on at least one csv with pages in its sibling directory.
func (c *Converter) looksLikeNotionExport() bool {
	idSuffixed := 0
	for _, entry := range c.mdEntries {
		if notionIdSuffix.MatchString(entry.Name) {
			idSuffixed++
		}
	}
	threshold := max(len(c.mdEntries)/5, 2)
	if idSuffixed >= threshold {
		return true
	}
	for _, entry := range c.csvEntries {
		if len(c.csvMembers(entry.Name)) > 0 {
			return true
		}
	}
	return false
}

// hasObsidianSegment reports whether a path lies inside an .obsidian config
// directory (the vault marker).
func hasObsidianSegment(dir string) bool {
	for segment := range strings.SplitSeq(dir, "/") {
		if segment == ".obsidian" {
			return true
		}
	}
	return false
}

// flavourIssue reports the resolved profile (§11.4 observability: "why did
// my import behave that way"). Silent for detected-generic — nothing
// flavour-specific is enabled there.
func (c *Converter) flavourIssue() (importv2.Issue, bool) {
	if c.flavour.Name == FlavourGeneric && !c.flavourForced {
		return importv2.Issue{}, false
	}
	how := "detected"
	if c.flavourForced {
		how = "requested"
	}
	message := fmt.Sprintf("markdown source %s as %s", how, c.flavour.Name)
	if clause := flavourEnables[c.flavour.Name]; clause != "" {
		message += "; " + clause
	}
	return importv2.Info(importv2.IssueFlavourDetected, message), true
}
