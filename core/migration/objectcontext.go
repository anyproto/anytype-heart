package migration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	time2 "github.com/anyproto/anytype-heart/util/time"
)

// fileContextTimeTolerance is the maximum allowed time difference (in seconds) between a file's
// creation and a potential context (block link or chat message). This accounts for the fact
// that file objects are created before the containing block/message is finalized.
const fileContextTimeTolerance = 5 * 60 // 5 minutes

// objectContextTimeTolerance is the window (in seconds) around an object's creation in which its context
// (block link or chat message) must have been created. Every path that creates an object through a link
// creates the link block in the same call, so a block from outside the window linked an object that
// already existed, or got the link later (e.g. a mention typed into an old paragraph).
const objectContextTimeTolerance = 60

// objectContextCutoff is the release of v0.50.0 (GO-7152), since which objects created through a link get
// createdInContext at creation. An object created after it without one was created standalone (sidebar,
// widget, set), so there is no context to backfill and any guess would be a false parent.
var objectContextCutoff = time.Date(2026, time.April, 28, 0, 0, 0, 0, time.UTC).Unix()

// importedOrigins mark objects created in bulk from snapshots. Their block ids were minted all at once by
// the import (or carried over from the source), so block timestamps can't tell which link created which object.
var importedOrigins = []model.ObjectOrigin{
	model.ObjectOrigin_import,
	model.ObjectOrigin_usecase,
	model.ObjectOrigin_builtin,
}

// systemRelationsToSkip contains system relations that should be skipped when building
// the incoming links map, as they are not meaningful for determining file creation context
var systemRelationsToSkip = []domain.RelationKey{
	bundle.RelationKeyCreator,
	bundle.RelationKeyLastModifiedBy,
	bundle.RelationKeyType,
	bundle.RelationKeyBacklinks,
	bundle.RelationKeyResolvedLayout,
	bundle.RelationKeyRecommendedFeaturedRelations,
	bundle.RelationKeyRecommendedRelations,
	bundle.RelationKeyRecommendedHiddenRelations,
	bundle.RelationKeySpaceId,
	bundle.RelationKeyIdentityProfileLink,
	bundle.RelationKeyTargetObjectType,
	bundle.RelationKeySourceObject,
}

/*
Context Migration Logic:

1. Make sure you Wait until all active syncing is done
   - Ensures all objects and their links are indexed before migration (done in spaceIndexer)

2. Find all files and GC-eligible objects without the CreatedInContext field set
   - Objects are limited to the ones created before objectContextCutoff and not imported
   - The owner and admins migrate every object, other members only the ones they created

3. For each target, find creation context:
   - Look up inbound links in anystore (we have indexed lookup for this)
   - Priority: earliest timed context (block link or chat message), then relation link fallback
   - blockId is BSON ObjectId containing timestamp, which has to fit the target's creation time:
     not later than it for files, within objectContextTimeTolerance of it for objects
   - The context object must be indexed, created before the target, and not newer than the block
     (an older block was copied in with its id, e.g. by a duplicate)
   - Set CreatedInContext = source objectId, CreatedInContextRef = blockId or messageId

4. The relation link fallback is a dry-run for objects: it is only logged, never written
*/

func (s *service) runObjectContextMigration(ctx context.Context, spaceId string, workspaceId string) error {
	spaceIndex := s.objectStore.SpaceIndex(spaceId)

	var l = log.With(zap.String("spaceId", spaceId), zap.Int("version", domain.MigrationObjectContextVersion))
	// Check if migration already done (version >= current)
	if s.isObjectContextMigrationDone(spaceIndex, workspaceId) {
		l.Debug("migration already done")
		return nil
	}

	l.Info("starting object context migration")

	gcLayouts := make([]int64, 0, len(domain.GCEligibleLayouts))
	for _, layout := range domain.GCEligibleLayouts {
		gcLayouts = append(gcLayouts, int64(layout))
	}

	// Step 1: Query all files and objects without context fields
	records, err := spaceIndex.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(gcLayouts),
			},
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_Empty,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query objects without context: %w", err)
	}

	l.With(zap.Int("count", len(records))).Debug("found objects without context")

	if len(records) == 0 {
		// persist the marker even with nothing to migrate, otherwise context-less spaces
		// re-enter the RunMigrationsWhenIdle polling loop on every load
		if err := s.markObjectContextMigrationDone(workspaceId); err != nil {
			l.Warn("failed to mark migration done", zap.Error(err))
		}
		return nil
	}

	myParticipantId := s.accountService.MyParticipantId(spaceId)
	migrateAll := s.canMigrateAllObjects(spaceIndex, myParticipantId)

	// Step 2: Build chat attachment index (single scan of all chats)
	chatAttachmentIndex, err := s.buildChatAttachmentIndex(ctx, spaceId, spaceIndex)
	if err != nil {
		l.Warn("failed to build chat attachment index", zap.Error(err))
		// Continue without chat context - not fatal
		chatAttachmentIndex = make(ChatAttachmentIndex)
	}
	l.Debug("built chat attachment index", zap.Int("attachments", len(chatAttachmentIndex)))

	// Step 3: Process each target - use indexed lookup for inbound links
	var stats contextMigrationStats
	for _, record := range records {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		target, ok := newContextTarget(record.Details)
		if !ok {
			stats.outOfScope++
			continue
		}
		if !migrateAll && record.Details.GetString(bundle.RelationKeyCreator) != myParticipantId {
			stats.notOwned++
			continue
		}

		tl := l.With(zap.String("objectId", target.id), zap.Bool("isFile", target.isFile), zap.Int64("createdAt", target.createdAt))
		contextInfo, err := s.resolveContext(spaceIndex, target, chatAttachmentIndex[target.id])
		if err != nil {
			tl.Warn("failed to resolve creation context", zap.Error(err))
			stats.failed++
			continue
		}
		if contextInfo == nil {
			tl.Debug("no creation context found")
			stats.notFound++
			continue
		}

		if contextInfo.kind == contextKindRelation && !target.isFile {
			// the relation fallback carries no timestamp, so for objects it only reports what it would
			// choose until the choices are validated. An empty relationKey is a collection membership.
			tl.Warn("object context migration dry-run: relation fallback context",
				zap.Int64("layout", record.Details.GetInt64(bundle.RelationKeyResolvedLayout)),
				zap.String("contextObjectId", contextInfo.objectId),
				zap.String("relationKey", contextInfo.relationKey),
				zap.Int64("contextCreatedAt", contextInfo.sourceCreated),
			)
			stats.dryRun++
			continue
		}

		if err := s.detailsService.SetCreatedInContextInternal(target.id, contextInfo.objectId, contextInfo.ref()); err != nil {
			if errors.Is(err, detailservice.ErrCreatedInContextAlreadySet) {
				// set at runtime or synced from another member since the query
				stats.alreadySet++
				continue
			}
			tl.Warn("failed to set creation context", zap.Error(err), zap.String("contextObjectId", contextInfo.objectId))
			stats.failed++
			continue
		}
		stats.migrated++
	}

	l.Info("completed object context migration",
		zap.Int("total", len(records)),
		zap.Bool("migrateAll", migrateAll),
		zap.Int("migrated", stats.migrated),
		zap.Int("dryRun", stats.dryRun),
		zap.Int("notFound", stats.notFound),
		zap.Int("outOfScope", stats.outOfScope),
		zap.Int("notOwned", stats.notOwned),
		zap.Int("alreadySet", stats.alreadySet),
		zap.Int("failed", stats.failed),
	)

	// Mark migration as done with current version. Failed targets don't hold it back: a failure that
	// repeats on every run would otherwise keep the space in the migration loop forever
	if err := s.markObjectContextMigrationDone(workspaceId); err != nil {
		l.Warn("failed to mark migration done", zap.Error(err))
	}

	return nil
}

type contextMigrationStats struct {
	migrated   int
	dryRun     int
	notFound   int
	outOfScope int
	notOwned   int
	alreadySet int
	failed     int
}

// contextTarget is a file or an object the migration looks up the creation context for
type contextTarget struct {
	id        string
	isFile    bool
	createdAt int64 // addedDate for files, createdDate for objects
}

// newContextTarget returns false when the object is out of the migration scope
func newContextTarget(details *domain.Details) (contextTarget, bool) {
	t := contextTarget{
		id:     details.GetString(bundle.RelationKeyId),
		isFile: slices.Contains(domain.FileLayouts, model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))),
	}
	if t.isFile {
		t.createdAt = details.GetInt64(bundle.RelationKeyAddedDate)
		return t, t.createdAt != 0
	}
	if slices.Contains(importedOrigins, model.ObjectOrigin(details.GetInt64(bundle.RelationKeyOrigin))) {
		return t, false
	}
	if details.GetInt64(bundle.RelationKeyAddedDate) != 0 {
		// an object gets addedDate only when created from a snapshot carrying its original creation date
		return t, false
	}
	t.createdAt = details.GetInt64(bundle.RelationKeyCreatedDate)
	return t, t.createdAt != 0 && t.createdAt < objectContextCutoff
}

func (t contextTarget) tolerance() int64 {
	if t.isFile {
		return fileContextTimeTolerance
	}
	return objectContextTimeTolerance
}

// acceptsContextTime reports whether a block or a chat message created at ts can be the target's context
func (t contextTarget) acceptsContextTime(ts int64) bool {
	if ts > t.createdAt+t.tolerance() {
		// created after the target, so it linked a target that already existed
		return false
	}
	// a file can be uploaded into a block made before it (an empty file block), while an object's
	// creating block is made in the same call as the object
	return t.isFile || ts >= t.createdAt-t.tolerance()
}

// acceptsSource returns the context object's creation date and whether it is indexed and was created
// before the target
func (t contextTarget) acceptsSource(sources map[string]int64, sourceId string) (int64, bool) {
	created, ok := sources[sourceId]
	if !ok {
		return 0, false
	}
	return created, created <= t.createdAt+t.tolerance()
}

type contextKind int

const (
	contextKindBlock contextKind = iota
	contextKindChat
	contextKindRelation
)

// contextInfo stores the resolved context for a target
type contextInfo struct {
	kind          contextKind
	objectId      string // context object ID (page, chat, etc.)
	blockId       string // block ID for block link contexts
	messageId     string // message ID for chat contexts
	relationKey   string // relation key for relation link contexts
	timestamp     int64  // 0 means no timestamp (relation link fallback)
	sourceCreated int64  // creation date of the context object
}

// ref returns the CreatedInContextRef value: the block or the chat message the target was created in
func (c *contextInfo) ref() string {
	if c.blockId != "" {
		return c.blockId
	}
	return c.messageId
}

// before orders block candidates: earliest block, then earliest context object, then ids for determinism
func (c *contextInfo) before(other *contextInfo) bool {
	if c.timestamp != other.timestamp {
		return c.timestamp < other.timestamp
	}
	if c.sourceCreated != other.sourceCreated {
		return c.sourceCreated < other.sourceCreated
	}
	if c.objectId != other.objectId {
		return c.objectId < other.objectId
	}
	return c.blockId < other.blockId
}

// resolveContext loads the target's inbound links and the creation dates of their sources, then finds
// the best context among them
func (s *service) resolveContext(spaceIndex spaceindex.Store, target contextTarget, chatCtx *ChatAttachmentContext) (*contextInfo, error) {
	inboundLinks, err := spaceIndex.GetInboundLinksDetailedById(target.id)
	if err != nil {
		return nil, fmt.Errorf("get inbound links: %w", err)
	}
	links := s.filterLinks(target.id, inboundLinks)

	sourceIds := make([]string, 0, len(links)+1)
	for _, link := range links {
		sourceIds = append(sourceIds, link.SourceID)
	}
	if chatCtx != nil {
		sourceIds = append(sourceIds, chatCtx.ChatObjectId)
	}
	if len(sourceIds) == 0 {
		return nil, nil
	}
	records, err := spaceIndex.QueryByIds(lo.Uniq(sourceIds))
	if err != nil {
		return nil, fmt.Errorf("query context objects: %w", err)
	}
	// a source missing here is not indexed, so it can't be a context
	sources := make(map[string]int64, len(records))
	for _, record := range records {
		sources[record.Details.GetString(bundle.RelationKeyId)] = record.Details.GetInt64(bundle.RelationKeyCreatedDate)
	}

	return s.findBestContext(target, links, chatCtx, sources), nil
}

// findBestContext finds the best creation context for a target among its filtered inbound links.
// sources maps every indexed context object to its creation date.
// Priority: earliest timed context (block link or chat), then relation link fallback
func (s *service) findBestContext(target contextTarget, links []spaceindex.IncomingLink, chatCtx *ChatAttachmentContext, sources map[string]int64) *contextInfo {
	// 1. Find earliest block link (has timestamp)
	blockCtx := s.findBlockContext(target, links, sources)

	// 2. Build chat context if valid
	chatInfo := s.findChatContext(target, chatCtx, sources)

	// 3. Pick earliest timed context
	if blockCtx != nil && chatInfo != nil {
		if blockCtx.timestamp <= chatInfo.timestamp {
			return blockCtx
		}
		return chatInfo
	}
	if blockCtx != nil {
		return blockCtx
	}
	if chatInfo != nil {
		return chatInfo
	}

	// 4. Fallback to relation link (no timestamp)
	return s.findRelationContext(target, links, sources)
}

// filterLinks removes self-references and system relations
func (s *service) filterLinks(targetId string, inboundLinks []spaceindex.IncomingLink) []spaceindex.IncomingLink {
	var links []spaceindex.IncomingLink
	for _, link := range inboundLinks {
		if link.SourceID == targetId {
			continue
		}
		if slices.Contains(systemRelationsToSkip, domain.RelationKey(link.RelationKey)) {
			continue
		}
		links = append(links, link)
	}
	return links
}

// findBlockContext finds the earliest valid block link context
func (s *service) findBlockContext(target contextTarget, links []spaceindex.IncomingLink, sources map[string]int64) *contextInfo {
	var earliest *contextInfo
	for _, link := range links {
		if link.BlockID == "" {
			continue
		}
		blockTs, ok := time2.BsonIdToTimestamp(link.BlockID)
		if !ok || !target.acceptsContextTime(blockTs) {
			continue
		}
		sourceCreated, ok := target.acceptsSource(sources, link.SourceID)
		if !ok {
			continue
		}
		if blockTs+target.tolerance() < sourceCreated {
			// the block is older than the object holding it, so it was copied there with its id
			// (duplicate, template): its timestamp says nothing about when the link appeared
			continue
		}
		candidate := &contextInfo{
			kind:          contextKindBlock,
			objectId:      link.SourceID,
			blockId:       link.BlockID,
			timestamp:     blockTs,
			sourceCreated: sourceCreated,
		}
		if earliest == nil || candidate.before(earliest) {
			earliest = candidate
		}
	}
	return earliest
}

// findChatContext returns the chat message the target was attached to, if it fits the target's creation time
func (s *service) findChatContext(target contextTarget, chatCtx *ChatAttachmentContext, sources map[string]int64) *contextInfo {
	if chatCtx == nil || !target.acceptsContextTime(chatCtx.CreatedAt) {
		return nil
	}
	sourceCreated, ok := sources[chatCtx.ChatObjectId]
	if !ok {
		return nil
	}
	return &contextInfo{
		kind:          contextKindChat,
		objectId:      chatCtx.ChatObjectId,
		messageId:     chatCtx.MessageId,
		timestamp:     chatCtx.CreatedAt,
		sourceCreated: sourceCreated,
	}
}

// findRelationContext finds the first valid relation link as fallback (no timestamp)
func (s *service) findRelationContext(target contextTarget, links []spaceindex.IncomingLink, sources map[string]int64) *contextInfo {
	relationLinks := lo.Filter(links, func(link spaceindex.IncomingLink, _ int) bool {
		return link.BlockID == ""
	})
	// Sort for determinism
	sort.Slice(relationLinks, func(i, j int) bool {
		if relationLinks[i].RelationKey != relationLinks[j].RelationKey {
			return relationLinks[i].RelationKey < relationLinks[j].RelationKey
		}
		return relationLinks[i].SourceID < relationLinks[j].SourceID
	})

	for _, link := range relationLinks {
		sourceCreated, ok := target.acceptsSource(sources, link.SourceID)
		if !ok {
			continue
		}
		return &contextInfo{
			kind:          contextKindRelation,
			objectId:      link.SourceID,
			relationKey:   link.RelationKey,
			sourceCreated: sourceCreated,
		}
	}
	return nil
}

// canMigrateAllObjects reports whether the participant migrates objects created by other members too. Only
// the owner and admins do, so that in a shared space the migration is not written by every member in
// parallel; other members migrate only the objects they created.
func (s *service) canMigrateAllObjects(spaceIndex spaceindex.Store, participantId string) bool {
	details, err := spaceIndex.GetDetails(participantId)
	if err != nil {
		return false
	}
	switch model.ParticipantPermissions(details.GetInt64(bundle.RelationKeyParticipantPermissions)) {
	case model.ParticipantPermissions_Owner, model.ParticipantPermissions_Admin:
		return true
	default:
		return false
	}
}

func (s *service) isObjectContextMigrationDone(spaceIndex spaceindex.Store, workspaceId string) bool {
	recs, err := spaceIndex.QueryByIds([]string{workspaceId})
	if err != nil || len(recs) == 0 {
		return false
	}
	storedVersion := recs[0].Details.GetInt64(bundle.RelationKeyMigrationObjectContext)
	return storedVersion >= domain.MigrationObjectContextVersion
}

func (s *service) markObjectContextMigrationDone(workspaceId string) error {
	// the workspace is space configuration: a plain member may not edit it, but every member runs
	// this migration, so it writes as the middleware rather than as the user
	return s.detailsService.SetDetailsInternal(workspaceId, []domain.Detail{
		{Key: bundle.RelationKeyMigrationObjectContext, Value: domain.Int64(domain.MigrationObjectContextVersion)},
	})
}
