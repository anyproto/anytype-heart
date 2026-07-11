package notion

import (
	"context"
	"fmt"
	"net/http"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/typesuggest"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const rootCollectionName = "Notion Import"

// lateDiscoveryCap bounds pass-2 discovery (§16 item 3): children the
// eventually-consistent /search index omitted are fetched and imported on
// demand, but a pathological workspace must not turn the drain into an
// unbounded crawl.
const lateDiscoveryCap = 1000

type Converter struct {
	client  *client.Client
	fetcher client.FileFetcher
	factory importv2.CollectionFactory
	tempDir string

	// pass-1 stubs (small strings): stream order and hierarchy knowledge.
	databases  []Entity
	pages      []Entity
	entityById map[string]Entity
	// dataSourcesByDatabase aliases the database ids that child_database
	// blocks and rich-text mentions still reference onto the imported
	// data-source entities.
	dataSourcesByDatabase map[string][]string

	properties      *propertiesStore
	files           *fileRegistry
	searchTruncated bool

	// pending queues entities discovered during pass 2 (second-chance
	// discovery): claimed on sight, converted by the Convert drain loop.
	pending        []Entity
	lateDiscovered int
	// discoveryMisses caches failed discovery fetches: an inaccessible id
	// referenced from many pages is attempted once per run, not once per
	// reference (each miss costs the client's full retry budget).
	discoveryMisses map[string]bool

	// suggestor types database rows that would otherwise import as plain
	// Pages (§11.5); suggestedTypes is keyed by data-source AND entity id
	// so both parent forms on a page stub resolve.
	suggestor      typesuggest.Suggestor
	suggestedTypes map[string]domain.TypeKey
}

// New builds a per-run converter. tempDir is the run-scoped download
// directory (removed by the adapter with the run).
func New(apiClient *client.Client, fetcher client.FileFetcher, factory importv2.CollectionFactory, tempDir string) *Converter {
	return &Converter{
		client:                apiClient,
		fetcher:               fetcher,
		factory:               factory,
		tempDir:               tempDir,
		entityById:            map[string]Entity{},
		dataSourcesByDatabase: map[string][]string{},
		discoveryMisses:       map[string]bool{},
		properties:            newPropertiesStore(),
		files:                 newFileRegistry(),
		suggestor:             typesuggest.NewNaive(),
		suggestedTypes:        map[string]domain.TypeKey{},
	}
}

func (c *Converter) Name() string { return "Notion" }

// EnumerateIdentities is the /search crawl: every page and data source the
// integration can see becomes one claim; only stubs are retained.
func (c *Converter) EnumerateIdentities(ctx context.Context, yield func(importv2.IdentityClaim) error) error {
	truncated, err := crawlSearch(ctx, c.client, func(entity Entity) error {
		if _, seen := c.entityById[entity.Id]; seen {
			return nil
		}
		c.entityById[entity.Id] = entity
		if entity.isCollectionLike() {
			c.databases = append(c.databases, entity)
			if entity.DatabaseId != "" {
				c.dataSourcesByDatabase[entity.DatabaseId] = append(c.dataSourcesByDatabase[entity.DatabaseId], entity.Id)
			}
		} else {
			c.pages = append(c.pages, entity)
		}
		return yield(importv2.IdentityClaim{
			SourceKey:      entity.Id,
			SbType:         coresb.SmartBlockTypePage,
			SourceFilePath: entity.Id,
		})
	})
	if err != nil {
		return fmt.Errorf("enumerate workspace: %w", err)
	}
	c.searchTruncated = truncated
	return nil
}

func (c *Converter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	if c.searchTruncated {
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, "search",
			"workspace search pagination stopped early (inconsistent cursor from the API); importing what was gathered"))
	}
	for _, database := range c.databases {
		if err := c.convertDatabase(ctx, database, sink); err != nil {
			return importv2.RootSpec{}, err
		}
	}
	// Pages pipeline: fetches run ahead in parallel (bounded, shared pacer),
	// emission stays in stub order on this goroutine. On an early return the
	// engine cancels the run context, which unblocks producer and workers.
	fetched := c.prefetchPages(ctx, c.pages, sink)
	for f := range fetched {
		select {
		case <-f.done:
		case <-ctx.Done():
			return importv2.RootSpec{}, ctx.Err()
		}
		if err := c.emitFetchedPage(ctx, f, sink); err != nil {
			return importv2.RootSpec{}, err
		}
	}
	// Drain second-chance discoveries (§16 item 3). The queue grows while it
	// drains — a late page's blocks may reference further omitted children.
	for i := 0; i < len(c.pending); i++ {
		entity := c.pending[i]
		var err error
		if entity.isCollectionLike() {
			err = c.convertDatabase(ctx, entity, sink)
		} else {
			err = c.convertPage(ctx, entity, sink)
		}
		if err != nil {
			return importv2.RootSpec{}, err
		}
	}
	return importv2.RootSpec{
		CollectionName: rootCollectionName,
		WidgetLayout:   model.BlockContentWidget_CompactList,
	}, nil
}

// discoverPage is the second-chance fetch for a page id referenced by a
// block but absent from the pass-1 claim set — the GO-5273 class: /search is
// eventually consistent and silently omits fresh or unlucky pages, yet the
// referencing block carries the child's exact fetchable id. A page the
// integration cannot access (404/403) stays unresolved and keeps the
// caller's missing-target degrade.
func (c *Converter) discoverPage(ctx context.Context, pageId string, sink importv2.Sink) (string, bool) {
	if pageId == "" || c.lateDiscovered >= lateDiscoveryCap || c.discoveryMisses[pageId] {
		return "", false
	}
	// GET /pages/{id} has the same stub shape /search results carry.
	var result searchResult
	if err := c.client.Request(ctx, http.MethodGet, "/pages/"+pageId, nil, &result); err != nil {
		c.discoveryMisses[pageId] = true
		return "", false
	}
	entity := Entity{Id: result.Id, Parent: result.Parent, Title: titleOf(result)}
	if entity.Id == "" || !c.adoptLateEntity(ctx, entity, sink) {
		return "", false
	}
	return entity.Id, true
}

// databaseStub is the GET /databases/{id} subset needed to adopt its data
// sources (collections are modeled per data source under the 2025-09-03 API).
type databaseStub struct {
	Id          string     `json:"id"`
	Title       []richText `json:"title"`
	Parent      Parent     `json:"parent"`
	DataSources []struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	} `json:"data_sources"`
}

// discoverDatabase is discoverPage's collection counterpart: child_database
// blocks and link_to_page reference the DATABASE id, which resolves through
// its data sources.
func (c *Converter) discoverDatabase(ctx context.Context, databaseId string, sink importv2.Sink) (string, bool) {
	if databaseId == "" || c.lateDiscovered >= lateDiscoveryCap || c.discoveryMisses[databaseId] {
		return "", false
	}
	var database databaseStub
	if err := c.client.Request(ctx, http.MethodGet, "/databases/"+databaseId, nil, &database); err != nil {
		c.discoveryMisses[databaseId] = true
		return "", false
	}
	for _, source := range database.DataSources {
		title := source.Name
		if title == "" {
			title = plainText(database.Title)
		}
		c.adoptLateEntity(ctx, Entity{
			Id:         source.Id,
			Kind:       kindDataSource,
			Title:      title,
			Parent:     database.Parent,
			DatabaseId: database.Id,
		}, sink)
	}
	return c.resolveDatabaseRef(databaseId)
}

// adoptLateEntity claims a pass-2 discovery and queues it for conversion by
// the Convert drain loop. Claim-before-reference is what lets the resolver
// treat the discovered id like any pass-1 identity.
func (c *Converter) adoptLateEntity(ctx context.Context, entity Entity, sink importv2.Sink) bool {
	if _, seen := c.entityById[entity.Id]; seen {
		return true
	}
	if c.lateDiscovered >= lateDiscoveryCap {
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, entity.Id,
			fmt.Sprintf("discovery cap (%d) reached; entity found outside search was not imported", lateDiscoveryCap)))
		return false
	}
	if err := sink.Claim(ctx, importv2.IdentityClaim{
		SourceKey:      entity.Id,
		SbType:         coresb.SmartBlockTypePage,
		SourceFilePath: entity.Id,
	}); err != nil {
		sink.Issue(importv2.Warning(importv2.IssueMissingTarget, entity.Id,
			fmt.Sprintf("claim discovered entity: %s", err)))
		return false
	}
	c.lateDiscovered++
	c.entityById[entity.Id] = entity
	if entity.isCollectionLike() && entity.DatabaseId != "" {
		c.dataSourcesByDatabase[entity.DatabaseId] = append(c.dataSourcesByDatabase[entity.DatabaseId], entity.Id)
	}
	c.pending = append(c.pending, entity)
	return true
}

// isRootCandidate mirrors v1's orphan rule: an entity joins the root
// collection when its parent is the workspace or was not itself imported
// (objects living inside an imported page/data source are reachable there).
func (c *Converter) isRootCandidate(entity Entity) bool {
	switch entity.Parent.Type {
	case "workspace":
		return true
	case "page_id":
		_, imported := c.entityById[entity.Parent.PageId]
		return !imported
	case "database_id":
		if _, imported := c.entityById[entity.Parent.DatabaseId]; imported {
			return false
		}
		return len(c.dataSourcesByDatabase[entity.Parent.DatabaseId]) == 0
	case "data_source_id":
		_, imported := c.entityById[entity.Parent.DataSourceId]
		return !imported
	case "block_id":
		// A block-parented entity lives inside some page's block tree and is
		// reachable via that page's child_page/child_database link. Which
		// page owns the block is unknowable from the pass-1 stubs (entityById
		// holds entity ids, never block ids), so mirror v1: block-parented is
		// never root — the alternative promoted every toggle/column-nested
		// page into the root collection.
		return false
	default:
		return entity.Parent.Workspace
	}
}

// resolveDatabaseRef maps a database id (the form child_database blocks,
// link_to_page and rich-text mentions still use) onto an imported entity:
// the id itself when imported, else the database's first data source (with
// more sources, the reference still lands on an imported collection).
func (c *Converter) resolveDatabaseRef(databaseId string) (string, bool) {
	if _, ok := c.entityById[databaseId]; ok {
		return databaseId, true
	}
	if sources := c.dataSourcesByDatabase[databaseId]; len(sources) > 0 {
		return sources[0], true
	}
	return "", false
}
