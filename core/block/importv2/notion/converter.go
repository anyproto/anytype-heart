package notion

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
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
	// so both parent forms on a page stub resolve. The suggestor only serves
	// late-discovered databases — pass-1 containers go through the plan.
	suggestor      typesuggest.Suggestor
	suggestedTypes map[string]domain.TypeKey

	// plan phase state (docs/ImportV2LLM.md): the planner sees every pass-1
	// schema at once; the sanitized plan drives container types and property
	// remaps for the containers in `planned`.
	planner        schemaplan.Planner
	includeSamples bool
	plan           schemaplan.Plan
	planned        map[string]bool
	planTypeKeys   map[domain.TypeKey]domain.TypeKey
	// typeBackedContainers are databases whose minted type took their place:
	// the type carries their source key, so they emit no collection of their own.
	// deferredTypes holds those types until their database's relations exist.
	typeBackedContainers map[string]bool
	deferredTypes        map[string]schemaplan.TypeDefinition
	// propertyScopes canonicalises a database's stub and data-source ids onto
	// one property scope (see propertyScope).
	propertyScopes map[string]string
	schemaFetches  map[string]*schemaFetch

	// skip marks entities a previous incarnation already recorded
	// (ResumableConverter, DM spec §8.3); nil on a fresh run. recoverKeys
	// are prior claims the spool never got (the seam's obligation half,
	// review P0-A) — re-fetched directly after the drain, because the skip
	// set suppresses re-walking the recorded parents that discovered them.
	// planReuse is the matching plan wiring: record on a fresh run, preset
	// on a resumed one (08-13 §6.3 — the plan is never recomputed).
	skip        func(sourceKey string) bool
	recoverKeys []string
	planReuse   schemaplan.Reuse
}

// Option configures a per-run converter.
type Option func(*Converter)

// WithPlanner injects the structure planner (the LLM one when the request
// carries provider config). Default is the naive typesuggest wrapper.
func WithPlanner(planner schemaplan.Planner) Option {
	return func(c *Converter) { c.planner = planner }
}

// WithContentSamples lets the planner see member page titles
// (request flag includeContentSamples).
func WithContentSamples() Option {
	return func(c *Converter) { c.includeSamples = true }
}

// WithPlanReuse wires the crawl-resume plan recording/reuse (08-13 §6.3).
func WithPlanReuse(reuse schemaplan.Reuse) Option {
	return func(c *Converter) { c.planReuse = reuse }
}

// SetSkip implements importv2.ResumableConverter: skip reports entities a
// previous incarnation already recorded, so the resumed crawl spends zero
// requests on them (the payoff case — each skipped page saves its page
// fetch and block-tree crawl; the re-search itself costs ~1 request per
// 100 entities).
func (c *Converter) SetSkip(skip func(sourceKey string) bool) { c.skip = skip }

// SetRecover implements the seam's obligation half (importv2
// .ResumableConverter, review P0-A): /search is eventually consistent, so
// enumeration is INCOMPLETE here — a prior claim that neither re-enumerates
// nor sits in the spool may still exist (a page found through a recorded
// parent's block tree, which the skip set now suppresses re-walking). Each
// key is re-fetched directly after the drain; see recoverUnrecorded.
func (c *Converter) SetRecover(unrecordedClaims []string) { c.recoverKeys = unrecordedClaims }

func (c *Converter) skipRecorded(id string) bool { return c.skip != nil && c.skip(id) }

// New builds a per-run converter. tempDir is the run-scoped download
// directory (removed by the adapter with the run).
func New(apiClient *client.Client, fetcher client.FileFetcher, factory importv2.CollectionFactory, tempDir string, opts ...Option) *Converter {
	c := &Converter{
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
		planner:               schemaplan.NewNaive(),
		planned:               map[string]bool{},
		planTypeKeys:          map[domain.TypeKey]domain.TypeKey{},
		typeBackedContainers:  map[string]bool{},
		deferredTypes:         map[string]schemaplan.TypeDefinition{},
		propertyScopes:        map[string]string{},
		schemaFetches:         map[string]*schemaFetch{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
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
	if err := c.planStructure(ctx, sink); err != nil {
		return importv2.RootSpec{}, err
	}
	for _, database := range c.databases {
		if err := c.convertDatabase(ctx, database, sink); err != nil {
			return importv2.RootSpec{}, err
		}
	}
	// Pages pipeline: fetches run ahead in parallel (bounded, shared pacer),
	// emission stays in stub order on this goroutine. On an early return the
	// engine cancels the run context, which unblocks producer and workers.
	// On a resumed crawl, recorded pages are dropped BEFORE the pipeline —
	// their stubs stay in entityById (hierarchy, titles, root-candidate
	// checks all keep working) but they cost zero requests; the engine's
	// replay serves their recorded objects. Databases are deliberately NOT
	// skipped above: rows converting in this incarnation need their property
	// mappings in converter memory, and a schema re-fetch is ~1 request per
	// data source (08-13 §6.3).
	pages := c.pages
	if c.skip != nil {
		pages = make([]Entity, 0, len(c.pages))
		for _, page := range c.pages {
			if c.skipRecorded(page.Id) {
				continue
			}
			pages = append(pages, page)
		}
	}
	fetched := c.prefetchPages(ctx, pages, sink)
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
	drained := 0
	if err := c.drainPending(ctx, sink, &drained); err != nil {
		return importv2.RootSpec{}, err
	}
	// Claim recovery (review P0-A, resumed crawls only): prior claims the
	// spool never got are re-fetched directly — the skip set suppressed
	// re-walking the recorded parents that discovered them, so the drain
	// alone cannot re-find them. Recovered entities join c.pending and
	// drain like any late discovery, their own children included: the loss
	// was transitive, so the recovery is too.
	if len(c.recoverKeys) > 0 {
		if err := c.recoverUnrecorded(ctx, sink); err != nil {
			return importv2.RootSpec{}, err
		}
		if err := c.drainPending(ctx, sink, &drained); err != nil {
			return importv2.RootSpec{}, err
		}
	}
	return importv2.RootSpec{
		CollectionName: rootCollectionName,
		WidgetLayout:   model.BlockContentWidget_CompactList,
	}, nil
}

// drainPending converts queued late discoveries from *drained onward,
// advancing the cursor so a later drain pass continues where this one
// stopped (the queue grows while it drains).
func (c *Converter) drainPending(ctx context.Context, sink importv2.Sink, drained *int) error {
	for ; *drained < len(c.pending); *drained++ {
		entity := c.pending[*drained]
		if !entity.isCollectionLike() && c.skipRecorded(entity.Id) {
			// A prior incarnation's late discovery, re-discovered and already
			// recorded: same skip rule as the pass-1 pages above. Collection-
			// like discoveries re-convert regardless (property mappings).
			continue
		}
		var err error
		if entity.isCollectionLike() {
			err = c.convertDatabase(ctx, entity, sink)
		} else {
			err = c.convertPage(ctx, entity, sink)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// recoverUnrecorded re-fetches every recovery key this incarnation has not
// re-encountered (review P0-A). The claim key IS the Notion id, so a direct
// GET settles each one three ways: alive → adopted and converted like any
// late discovery; positively gone (404/403 from the API) → an honest
// data-loss warning; anything else → a loud object failure that keeps its
// retryable shape — never a drift claim nobody established. The STOP is the
// fourth way out and it is not a verdict at all: it aborts the crawl.
func (c *Converter) recoverUnrecorded(ctx context.Context, sink importv2.Sink) error {
	for _, key := range c.recoverKeys {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, seen := c.entityById[key]; seen {
			continue // re-enumerated by /search, or re-discovered from an unrecorded parent
		}
		if err := c.recoverOne(ctx, key, sink); err != nil {
			return err
		}
	}
	return nil
}

// recoverOne probes ONE claim key through every id shape pass 1 can claim.
// /search yields pages, data sources AND bare databases (search.go's three
// entity kinds), and EnumerateIdentities claims all three under the entity's
// own id — so a ladder that stops at two rungs 404s on a live database and
// reports it deleted, which is verbatim the symptom the recovery seam exists
// to remove (review item 2).
//
// It returns an error only for the STOP. A probe the run's cancellation
// killed proves nothing about the entity, and the classification below used
// to read the error's SHAPE without consulting ctx.Err() (review item 1): a
// cancelled import then reported a retryable-shaped object failure, so the
// settlement kept its dir — token intact — and the next start silently
// re-ran the import the user had discarded.
func (c *Converter) recoverOne(ctx context.Context, key string, sink importv2.Sink) error {
	// GET /pages/{id} carries the same stub shape /search results do.
	var page searchResult
	pageErr := c.client.Request(ctx, http.MethodGet, "/pages/"+key, nil, &page)
	if pageErr == nil && page.Id != "" {
		// Adoption failures (discovery cap, claim error) issue their own
		// warnings under this key — reconciliation stays satisfied.
		c.adoptLateEntity(ctx, Entity{Id: page.Id, Parent: page.Parent, Title: titleOf(page)}, sink)
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Not fetchable as a page — a late-discovered DATA SOURCE has the same
	// claim shape. Resolve it through its owning database: the proven
	// discovery path, proper parents included, and it adopts sibling data
	// sources exactly as the original discovery did.
	var source struct {
		Id     string `json:"id"`
		Parent Parent `json:"parent"`
	}
	sourceErr := c.client.Request(ctx, http.MethodGet, "/data_sources/"+key, nil, &source)
	if sourceErr == nil {
		if source.Parent.DatabaseId != "" {
			c.discoverDatabase(ctx, source.Parent.DatabaseId, sink)
			if _, seen := c.entityById[key]; seen {
				return nil
			}
		}
		// The data source EXISTS but could not be adopted (its database
		// fetch failed, the cap, an odd parent shape): loud failure below —
		// classifying a confirmed-alive entity as drift would be the exact
		// lie this path exists to remove.
		sink.Issue(importv2.ObjectError(importv2.IssueObjectFailed, key,
			fmt.Errorf("recover claimed data source: could not adopt via its database")))
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// The third shape: a bare DATABASE result. Pass 1 claims it under the
	// database id and pass 2 converts it as a collection-like stub whose
	// schema resolves through its first data source (fetchSchema's
	// kindDatabase branch) — so recovery adopts it in exactly that form,
	// under the id that was claimed.
	var database databaseStub
	dbErr := c.client.Request(ctx, http.MethodGet, "/databases/"+key, nil, &database)
	if dbErr == nil && database.Id != "" {
		c.adoptLateEntity(ctx, Entity{
			Id:         database.Id,
			Kind:       kindDatabase,
			Title:      plainText(database.Title),
			Parent:     database.Parent,
			DatabaseId: database.Id,
		}, sink)
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if claimGone(pageErr) && claimGone(sourceErr) && claimGone(dbErr) {
		// POSITIVE not-found from the API, every shape: the source no longer
		// offers the entity — deleted, or no longer shared with the
		// integration. This is the honest drift report (08-13 §5.4).
		sink.Issue(importv2.Warning(importv2.IssueDataLoss, key,
			"object found by an interrupted import session no longer exists in Notion (or is no longer shared with the integration); it was not imported"))
		return nil
	}
	// Transport trouble, rate limits, 5xx: the entity may well still exist.
	// Loud, and the wrapped cause keeps its retryable shape so an
	// all-or-nothing abort classifies as transient and keeps the dir.
	sink.Issue(importv2.ObjectError(importv2.IssueObjectFailed, key,
		fmt.Errorf("re-fetch claimed object: %w", errors.Join(pageErr, sourceErr, dbErr))))
	return nil
}

// claimGone reports a positive the-source-no-longer-offers-it answer: 404
// (deleted) or 403 (sharing revoked). Anything else — transport failures,
// rate limits, 5xx — proves nothing about the entity.
func claimGone(err error) bool {
	return errors.Is(err, client.ErrNotFound) || errors.Is(err, client.ErrForbidden)
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
