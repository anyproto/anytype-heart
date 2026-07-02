package notion

import (
	"context"
	"fmt"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
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

	properties *propertiesStore
	files      *fileRegistry
}

// New builds a per-run converter. tempDir is the run-scoped download
// directory (removed by the adapter with the run).
func New(apiClient *client.Client, fetcher client.FileFetcher, factory importv2.CollectionFactory, tempDir string) *Converter {
	return &Converter{
		client:     apiClient,
		fetcher:    fetcher,
		factory:    factory,
		tempDir:    tempDir,
		entityById: map[string]Entity{},
		properties: newPropertiesStore(),
		files:      newFileRegistry(),
	}
}

func (c *Converter) Name() string { return "Notion" }

// EnumerateIdentities is the /search crawl: every page and database the
// integration can see becomes one claim; only stubs are retained.
func (c *Converter) EnumerateIdentities(ctx context.Context, yield func(importv2.IdentityClaim) error) error {
	err := crawlSearch(ctx, c.client, func(entity Entity) error {
		if _, seen := c.entityById[entity.Id]; seen {
			return nil
		}
		c.entityById[entity.Id] = entity
		if entity.IsDatabase {
			c.databases = append(c.databases, entity)
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
	return nil
}

func (c *Converter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
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
// (objects living inside an imported page/database are reachable there).
func (c *Converter) isRootCandidate(entity Entity) bool {
	switch entity.Parent.Type {
	case "workspace":
		return true
	case "page_id":
		_, imported := c.entityById[entity.Parent.PageId]
		return !imported
	case "database_id":
		_, imported := c.entityById[entity.Parent.DatabaseId]
		return !imported
	case "block_id":
		_, imported := c.entityById[entity.Parent.BlockId]
		return !imported
	default:
		return entity.Parent.Workspace
	}
}
