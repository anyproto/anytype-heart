// Package markdown is the streaming markdown/Obsidian converter: one page is
// converted and released at a time; the only converter-resident state is the
// listing metadata, emitted-definition dedup sets and the deterministic
// property-key resolver. See docs/ImportV2Design.md §11.1.
//
// Increment 1 covers: YAML front-matter → relations/options/types, anymark
// block conversion, H1 title extraction, link/mention/file/bookmark
// rewriting with source-key targets, per-directory CSV collections, and the
// root collection spec. JSON schema files, directory pages and
// properties-as-blocks land in increment 2.
package markdown

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const rootCollectionName = "Markdown Import"

type Params struct {
	// CreateDirectoryPages and IncludePropertiesAsBlock arrive in
	// increment 2; carried here so the adapter contract is stable.
	CreateDirectoryPages     bool
	IncludePropertiesAsBlock bool
}

type Converter struct {
	source  source.Source
	params  Params
	factory importv2.CollectionFactory

	resolver *mdResolver
	schemas  *schemaSet // nil when the source carries no anytype schemas
	dirs     *dirTree   // nil unless directory pages are enabled
	// listing metadata (small strings), built during pass 1.
	mdEntries  []source.Entry
	csvEntries []source.Entry
	baseNames  map[string][]string // base name → full entry names

	emittedRelations map[string]bool   // relation key
	emittedOptions   map[string]bool   // option source key
	emittedTypes     map[string]string // type name → type key
	emittedFiles     map[string]bool   // file entry name
}

// New builds a per-run converter instance (never shared between runs).
func New(src source.Source, params Params, factory importv2.CollectionFactory) *Converter {
	return &Converter{
		source:           src,
		params:           params,
		factory:          factory,
		baseNames:        map[string][]string{},
		emittedRelations: map[string]bool{},
		emittedOptions:   map[string]bool{},
		emittedTypes:     map[string]string{},
		emittedFiles:     map[string]bool{},
	}
}

// Schemas force-disable directory pages and properties-as-blocks (v1 rule).
func (c *Converter) directoryPagesEnabled() bool {
	return c.params.CreateDirectoryPages && c.schemas == nil
}

func (c *Converter) propertiesAsBlockEnabled() bool {
	return c.params.IncludePropertiesAsBlock && c.schemas == nil
}

func (c *Converter) Name() string { return "Markdown" }

func (c *Converter) EnumerateIdentities(ctx context.Context, yield func(importv2.IdentityClaim) error) error {
	schemas, err := loadSchemas(ctx, c.source)
	if err != nil {
		return fmt.Errorf("load schemas: %w", err)
	}
	c.schemas = schemas
	c.resolver = newResolver(schemas)

	err = c.source.Walk(ctx, func(e source.Entry) error {
		c.baseNames[path.Base(e.Name)] = append(c.baseNames[path.Base(e.Name)], e.Name)
		switch strings.ToLower(path.Ext(e.Name)) {
		case ".md":
			c.mdEntries = append(c.mdEntries, e)
		case ".csv":
			c.csvEntries = append(c.csvEntries, e)
		default:
			return nil
		}
		return yield(importv2.IdentityClaim{
			SourceKey:      e.Name,
			SbType:         coresb.SmartBlockTypePage,
			SourceFilePath: sourcePathHash(e.Name),
		})
	})
	if err != nil {
		return fmt.Errorf("walk source: %w", err)
	}
	if len(c.mdEntries) == 0 {
		return importv2.Fatal(importv2.IssueNoObjects, fmt.Errorf("no markdown files in source"))
	}

	if c.directoryPagesEnabled() {
		c.dirs = buildDirTree(append(append([]source.Entry{}, c.mdEntries...), c.csvEntries...))
		for _, dir := range c.dirs.dirs {
			if err := yield(importv2.IdentityClaim{
				SourceKey:      dirSourceKey(dir),
				SbType:         coresb.SmartBlockTypePage,
				SourceFilePath: sourcePathHash(dirSourceKey(dir)),
			}); err != nil {
				return fmt.Errorf("claim directory page: %w", err)
			}
		}
	}
	return nil
}

func (c *Converter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	for _, rejected := range c.source.Rejected() {
		sink.Issue(importv2.Warning(importv2.IssueSourceInvalid, rejected,
			"archive entry rejected: path escapes the archive root"))
	}
	if err := c.emitSchemaDefinitions(ctx, sink); err != nil {
		return importv2.RootSpec{}, err
	}
	for _, entry := range c.mdEntries {
		if err := c.convertPage(ctx, entry, sink); err != nil {
			return importv2.RootSpec{}, err
		}
	}
	for _, entry := range c.csvEntries {
		if err := c.convertCsvCollection(ctx, entry, sink); err != nil {
			return importv2.RootSpec{}, err
		}
	}
	if c.dirs != nil {
		if err := c.emitDirectoryPages(ctx, sink); err != nil {
			return importv2.RootSpec{}, err
		}
		// The root directory page is the import's entry point: a Tree
		// widget targets it directly, no wrapper collection (v1's
		// single-directory-page case).
		return importv2.RootSpec{
			RootObjectKey: dirSourceKey(c.dirs.root),
			WidgetLayout:  model.BlockContentWidget_Tree,
		}, nil
	}
	return importv2.RootSpec{
		CollectionName: rootCollectionName,
		WidgetLayout:   model.BlockContentWidget_Link,
	}, nil
}

// convertCsvCollection emits one collection per .csv file whose members are
// the sibling-directory markdown pages (the Notion-export `Foo.csv` ↔ `Foo/`
// convention). CSV rows are not parsed — v1 parity.
func (c *Converter) convertCsvCollection(ctx context.Context, entry source.Entry, sink importv2.Sink) error {
	members := c.csvMembers(entry.Name)
	title := pageTitleFromPath(entry.Name)
	object, err := c.factory.MakeCollection(title, members)
	if err != nil {
		return fmt.Errorf("make csv collection %q: %w", entry.Name, err)
	}
	object.SourceKey = entry.Name
	object.IsRootCandidate = isTopLevel(entry.Name)
	c.stampCommonDetails(object, entry, title)
	return sink.Object(ctx, object)
}

// csvMembers lists markdown pages living in the directory named after the
// csv file, in listing order.
func (c *Converter) csvMembers(csvName string) []string {
	dir := strings.TrimSuffix(csvName, path.Ext(csvName))
	var members []string
	for _, entry := range c.mdEntries {
		if path.Dir(entry.Name) == dir {
			members = append(members, entry.Name)
		}
	}
	return members
}

// lookupEntry resolves a link target (converter-relative path, possibly
// URL-escaped) to a source entry name. Basename fallback fires only on a
// unique match — v1 silently picked the last same-named file.
func (c *Converter) lookupEntry(target string) (string, bool) {
	name := source.NormalizeName(target)
	if _, ok := c.source.Stat(name); ok {
		return name, true
	}
	if unescaped, err := url.PathUnescape(name); err == nil && unescaped != name {
		unescaped = source.NormalizeName(unescaped)
		if _, ok := c.source.Stat(unescaped); ok {
			return unescaped, true
		}
		name = unescaped
	}
	if candidates, ok := c.baseNames[path.Base(name)]; ok && len(candidates) == 1 {
		return candidates[0], true
	}
	return name, false
}

func isTopLevel(name string) bool {
	return !strings.Contains(name, "/")
}

func pageTitleFromPath(name string) string {
	base := path.Base(name)
	return strings.TrimSuffix(base, path.Ext(base))
}

func (c *Converter) openEntry(name string) func(ctx context.Context) (io.ReadCloser, error) {
	return func(ctx context.Context) (io.ReadCloser, error) {
		return c.source.Open(ctx, name)
	}
}
