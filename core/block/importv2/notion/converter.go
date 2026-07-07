package notion

import (
	"context"
	"fmt"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/typesuggest"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const rootCollectionName = "Notion Import"

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
	for _, page := range c.pages {
		if err := c.convertPage(ctx, page, sink); err != nil {
			return importv2.RootSpec{}, err
		}
	}
	return importv2.RootSpec{
		CollectionName: rootCollectionName,
		WidgetLayout:   model.BlockContentWidget_CompactList,
	}, nil
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
