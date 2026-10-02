package migration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	time2 "github.com/anyproto/anytype-heart/util/time"
)

// fileContextTimeTolerance is the maximum allowed time difference (in seconds) between a file's
// creation and a potential context (block link or chat message). This accounts for the fact
// that file objects are created before the containing block/message is finalized.
const fileContextTimeTolerance = 5 * 60 // 5 minutes

// objectContextTimeTolerance is how much later (in seconds) than an object its context object may have been
// created, which absorbs the clock skew between the devices that created them.
const objectContextTimeTolerance = 60

// contextWindow is how long (in seconds) before and after its target a context may have been created
type contextWindow struct {
	before, after int64
}

// contextWindows bound, per kind, when the context of an object was created relative to the object. Every
// path that creates an object through a link, a chat or a collection does it right where the object is
// created, so a context from outside the window linked an object that already existed.
//   - block: the link block is created in the same call as the object. A bookmark block may come up to a
//     minute before its object, which is created once the page is fetched.
//   - chat: a message is often sent within a minute after its attachment is created.
//   - collection: an object created in a collection view is added to it in the same call. It applies to
//     files too, while their blocks and messages are bounded by fileContextTimeTolerance alone.
var contextWindows = map[contextKind]contextWindow{
	contextKindBlock:      {before: 60, after: 30},
	contextKindChat:       {before: 60, after: 60},
	contextKindCollection: {before: 10, after: 10},
}

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

var chatLayouts = []model.ObjectTypeLayout{
	model.ObjectType_chatDerived,
	model.ObjectType_chatDeprecated,
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
   - The context object must be an indexed content object with a creation date, created before the target
   - Priority: earliest timed context, which has to fit the target's creation time (see contextWindows):
     - a link, file, bookmark or inline set block pointing at the target. blockId is BSON ObjectId
       containing timestamp, and the block must not be older than its object (copied in with its id)
     - a chat message the target is attached to
   - Then the collection the target was added to by its creator when created, read from its history
   - Then, for files only, a relation link
   - Set CreatedInContext = source objectId, CreatedInContextRef = blockId or messageId
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
	reader := s.newContextReader(ctx, spaceId)
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
		if !migrateAll && target.creator != myParticipantId {
			stats.notOwned++
			continue
		}

		tl := l.With(zap.String("objectId", target.id), zap.Bool("isFile", target.isFile), zap.Int64("createdAt", target.createdAt))
		contextInfo, err := s.resolveContext(spaceIndex, target, chatAttachmentIndex[target.id], reader)
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
		zap.Int("notFound", stats.notFound),
		zap.Int("outOfScope", stats.outOfScope),
		zap.Int("notOwned", stats.notOwned),
		zap.Int("alreadySet", stats.alreadySet),
		zap.Int("failed", stats.failed),
		zap.Int("collectionsRead", reader.collectionsRead),
		zap.Duration("collectionReadTime", reader.collectionReadTime),
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
	creator   string
	createdAt int64 // addedDate for files, createdDate for objects
}

// newContextTarget returns false when the object is out of the migration scope
func newContextTarget(details *domain.Details) (contextTarget, bool) {
	t := contextTarget{
		id:      details.GetString(bundle.RelationKeyId),
		isFile:  slices.Contains(domain.FileLayouts, model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))),
		creator: details.GetString(bundle.RelationKeyCreator),
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

// acceptsContextTime reports whether a context of the kind created at ts can be the target's context
func (t contextTarget) acceptsContextTime(kind contextKind, ts int64) bool {
	if t.isFile && kind != contextKindCollection {
		// created after the file, so it linked a file that already existed. No lower bound: a file can be
		// uploaded into a block made before it (an empty file block)
		return ts <= t.createdAt+fileContextTimeTolerance
	}
	window := contextWindows[kind]
	return ts >= t.createdAt-window.before && ts <= t.createdAt+window.after
}

// sourceInfo is what the index tells about a possible context object
type sourceInfo struct {
	createdDate int64
	layout      model.ObjectTypeLayout
}

// acceptsSource returns the context object and whether it can be the target's context: an indexed content
// object with a creation date, created before the target. Types, the space and other system objects link
// objects without being where they were created, and some of them have no creation date to check.
func (t contextTarget) acceptsSource(sources map[string]sourceInfo, sourceId string) (sourceInfo, bool) {
	source, ok := sources[sourceId]
	if !ok || source.createdDate == 0 || !slices.Contains(domain.GCEligibleLayouts, source.layout) {
		return source, false
	}
	return source, source.createdDate <= t.createdAt+t.tolerance()
}

type contextKind int

const (
	contextKindBlock contextKind = iota
	contextKindChat
	contextKindCollection
	contextKindRelation
)

// contextInfo stores the resolved context for a target
type contextInfo struct {
	kind          contextKind
	objectId      string // context object ID (page, chat, collection, etc.)
	blockId       string // block ID for block link contexts
	messageId     string // message ID for chat contexts
	relationKey   string // relation key for relation link contexts
	timestamp     int64  // 0 means no timestamp (relation link fallback)
	sourceCreated int64  // creation date of the context object
}

// ref returns the CreatedInContextRef value: the block or the chat message the target was created in.
// It is empty for collections, like for the objects clients create in a collection view.
func (c *contextInfo) ref() string {
	if c.blockId != "" {
		return c.blockId
	}
	return c.messageId
}

// before orders candidates: earliest context, then earliest context object, then ids for determinism
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

// resolveContext loads the target's inbound links and the context objects they come from, then finds
// the best context among them
func (s *service) resolveContext(spaceIndex spaceindex.Store, target contextTarget, chatCtx *ChatAttachmentContext, reader *contextReader) (*contextInfo, error) {
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
	sources := make(map[string]sourceInfo, len(records))
	for _, record := range records {
		sources[record.Details.GetString(bundle.RelationKeyId)] = sourceInfo{
			createdDate: record.Details.GetInt64(bundle.RelationKeyCreatedDate),
			layout:      model.ObjectTypeLayout(record.Details.GetInt64(bundle.RelationKeyResolvedLayout)),
		}
	}

	return s.findBestContext(target, links, chatCtx, sources, reader), nil
}

// findBestContext finds the best creation context for a target among its filtered inbound links.
// sources maps every indexed context object to what the index tells about it.
// Priority: earliest timed context (block link or chat), then collection, then relation link fallback for files
func (s *service) findBestContext(target contextTarget, links []spaceindex.IncomingLink, chatCtx *ChatAttachmentContext, sources map[string]sourceInfo, reader *contextReader) *contextInfo {
	// 1. Find earliest block link (has timestamp)
	blockCtx := s.findBlockContext(target, links, sources, reader)

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

	// 4. Collection the target was created in. It has no ref to tie the target to, so it only fills in,
	// which also spares reading the history of collections whose members have a block or chat context
	if collectionCtx := s.findCollectionContext(target, links, sources, reader); collectionCtx != nil {
		return collectionCtx
	}

	// 5. Fallback to relation link (no timestamp). Objects are linked by relations they were not created
	// in (assignee, template, project), so only files use it: an image is uploaded as the icon or cover
	if target.isFile {
		return s.findRelationContext(target, links, sources)
	}
	return nil
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
func (s *service) findBlockContext(target contextTarget, links []spaceindex.IncomingLink, sources map[string]sourceInfo, reader *contextReader) *contextInfo {
	var candidates []*contextInfo
	for _, link := range links {
		if link.BlockID == "" {
			continue
		}
		blockTs, ok := time2.BsonIdToTimestamp(link.BlockID)
		if !ok || !target.acceptsContextTime(contextKindBlock, blockTs) {
			continue
		}
		source, ok := target.acceptsSource(sources, link.SourceID)
		if !ok {
			continue
		}
		if blockTs+target.tolerance() < source.createdDate {
			// the block is older than the object holding it, so it was copied there with its id
			// (duplicate, template): its timestamp says nothing about when the link appeared
			continue
		}
		candidates = append(candidates, &contextInfo{
			kind:          contextKindBlock,
			objectId:      link.SourceID,
			blockId:       link.BlockID,
			timestamp:     blockTs,
			sourceCreated: source.createdDate,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].before(candidates[j])
	})
	for _, candidate := range candidates {
		// the index records the block of a text mention too, and mentioning an object doesn't create it
		if reader.blockLinksTarget(candidate.objectId, candidate.blockId, target.id) {
			return candidate
		}
	}
	return nil
}

// findChatContext returns the chat message the target was attached to, if it fits the target's creation time
func (s *service) findChatContext(target contextTarget, chatCtx *ChatAttachmentContext, sources map[string]sourceInfo) *contextInfo {
	if chatCtx == nil || !target.acceptsContextTime(contextKindChat, chatCtx.CreatedAt) {
		return nil
	}
	// the creation date of a chat is not checked: it can be later than its first messages
	source, ok := sources[chatCtx.ChatObjectId]
	if !ok || !slices.Contains(chatLayouts, source.layout) {
		return nil
	}
	return &contextInfo{
		kind:          contextKindChat,
		objectId:      chatCtx.ChatObjectId,
		messageId:     chatCtx.MessageId,
		timestamp:     chatCtx.CreatedAt,
		sourceCreated: source.createdDate,
	}
}

// findCollectionContext finds the collection the target was created in: the one its creator added it to
// right when it was created
func (s *service) findCollectionContext(target contextTarget, links []spaceindex.IncomingLink, sources map[string]sourceInfo, reader *contextReader) *contextInfo {
	var earliest *contextInfo
	for _, link := range links {
		// a collection links its members with neither a block nor a relation
		if link.BlockID != "" || link.RelationKey != "" {
			continue
		}
		source, ok := target.acceptsSource(sources, link.SourceID)
		if !ok || source.layout != model.ObjectType_collection {
			continue
		}
		add, ok := reader.collectionAdd(link.SourceID, target.id)
		if !ok || add.participantId != target.creator || !target.acceptsContextTime(contextKindCollection, add.timestamp) {
			continue
		}
		candidate := &contextInfo{
			kind:          contextKindCollection,
			objectId:      link.SourceID,
			timestamp:     add.timestamp,
			sourceCreated: source.createdDate,
		}
		if earliest == nil || candidate.before(earliest) {
			earliest = candidate
		}
	}
	return earliest
}

// findRelationContext finds the first valid relation link as fallback (no timestamp)
func (s *service) findRelationContext(target contextTarget, links []spaceindex.IncomingLink, sources map[string]sourceInfo) *contextInfo {
	relationLinks := lo.Filter(links, func(link spaceindex.IncomingLink, _ int) bool {
		// collection membership is left to findCollectionContext, which checks when it happened
		return link.BlockID == "" && link.RelationKey != ""
	})
	// Sort for determinism
	sort.Slice(relationLinks, func(i, j int) bool {
		if relationLinks[i].RelationKey != relationLinks[j].RelationKey {
			return relationLinks[i].RelationKey < relationLinks[j].RelationKey
		}
		return relationLinks[i].SourceID < relationLinks[j].SourceID
	})

	for _, link := range relationLinks {
		source, ok := target.acceptsSource(sources, link.SourceID)
		if !ok {
			continue
		}
		return &contextInfo{
			kind:          contextKindRelation,
			objectId:      link.SourceID,
			relationKey:   link.RelationKey,
			sourceCreated: source.createdDate,
		}
	}
	return nil
}

// collectionAdd is the first change that added an object to a collection
type collectionAdd struct {
	timestamp     int64
	participantId string
}

// contextReader reads what the index doesn't tell about context objects: their blocks and the history of
// collections. It reads the history of each collection once per migration run.
type contextReader struct {
	linksTarget    func(objectId, blockId, targetId string) (bool, error)
	readCollection func(collectionId string) (map[string]collectionAdd, error)

	collections        map[string]map[string]collectionAdd
	collectionsRead    int
	collectionReadTime time.Duration
}

func newContextReader(
	linksTarget func(objectId, blockId, targetId string) (bool, error),
	readCollection func(collectionId string) (map[string]collectionAdd, error),
) *contextReader {
	return &contextReader{
		linksTarget:    linksTarget,
		readCollection: readCollection,
		collections:    make(map[string]map[string]collectionAdd),
	}
}

// contextReaderFor returns a contextReader over the objects and the trees of the space
func (s *service) contextReaderFor(ctx context.Context, spaceId string) *contextReader {
	return newContextReader(
		func(objectId, blockId, targetId string) (bool, error) {
			return s.blockLinksTarget(ctx, objectId, blockId, targetId)
		},
		func(collectionId string) (map[string]collectionAdd, error) {
			return s.readCollectionAdds(ctx, spaceId, collectionId)
		},
	)
}

func (r *contextReader) blockLinksTarget(objectId, blockId, targetId string) bool {
	links, err := r.linksTarget(objectId, blockId, targetId)
	if err != nil {
		log.Debug("failed to read context block", zap.String("objectId", objectId), zap.String("blockId", blockId), zap.Error(err))
		return false
	}
	return links
}

func (r *contextReader) collectionAdd(collectionId, memberId string) (collectionAdd, bool) {
	adds, read := r.collections[collectionId]
	if !read {
		start := time.Now()
		var err error
		adds, err = r.readCollection(collectionId)
		elapsed := time.Since(start)
		r.collectionsRead++
		r.collectionReadTime += elapsed
		if err != nil {
			log.Warn("failed to read collection history", zap.String("collectionId", collectionId), zap.Error(err))
		} else {
			log.Debug("read collection history", zap.String("collectionId", collectionId), zap.Int("members", len(adds)), zap.Duration("elapsed", elapsed))
		}
		// a failed read is remembered too, so it is not repeated for every member
		r.collections[collectionId] = adds
	}
	add, ok := adds[memberId]
	return add, ok
}

// blockLinksTarget reports whether the block of the object is a link, file, bookmark or inline set block
// pointing at the target
func (s *service) blockLinksTarget(ctx context.Context, objectId, blockId, targetId string) (bool, error) {
	var links bool
	err := cache.DoContext(s.objectGetter, ctx, objectId, func(sb smartblock.SmartBlock) error {
		if b := sb.Pick(blockId); b != nil {
			links = blockLinksTarget(b.Model(), targetId)
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("get object: %w", err)
	}
	return links, nil
}

func blockLinksTarget(b *model.Block, targetId string) bool {
	switch {
	case b.GetLink() != nil:
		return b.GetLink().TargetBlockId == targetId
	case b.GetFile() != nil:
		return b.GetFile().TargetObjectId == targetId
	case b.GetBookmark() != nil:
		return b.GetBookmark().TargetObjectId == targetId
	case b.GetDataview() != nil:
		return b.GetDataview().TargetObjectId == targetId
	default:
		return false
	}
}

// readCollectionAdds returns the first change that added each member of the collection. It reads the full
// history from local storage like version history does: the live tree may start at a snapshot made after
// the add.
func (s *service) readCollectionAdds(ctx context.Context, spaceId, collectionId string) (map[string]collectionAdd, error) {
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	treeBuilder := spc.TreeBuilder()
	if treeBuilder == nil {
		return nil, fmt.Errorf("space has no tree builder")
	}
	tree, err := treeBuilder.BuildHistoryTree(ctx, collectionId, objecttreebuilder.HistoryTreeOpts{})
	if err != nil {
		return nil, fmt.Errorf("build history tree: %w", err)
	}

	adds := make(map[string]collectionAdd)
	err = tree.IterateRoot(unmarshalCollectionAdds, func(change *objecttree.Change) bool {
		ids, _ := change.Model.([]string)
		for _, id := range ids {
			if prev, ok := adds[id]; ok && prev.timestamp <= change.Timestamp {
				continue
			}
			add := collectionAdd{timestamp: change.Timestamp}
			if change.Identity != nil {
				add.participantId = domain.NewParticipantId(spaceId, change.Identity.Account())
			}
			adds[id] = add
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("iterate history tree: %w", err)
	}
	return adds, nil
}

// unmarshalCollectionAdds decodes a change of a collection into just the ids it adds to the collection, so
// the history tree doesn't hold every decoded change: each snapshot carries the whole state
func unmarshalCollectionAdds(treeChange *objecttree.Change, data []byte) (any, error) {
	res, err := sourceimpl.UnmarshalChange(treeChange, data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal change: %w", err)
	}
	change, ok := res.(*pb.Change)
	if !ok {
		return nil, nil
	}
	ids := pbtypes.GetStringList(change.GetSnapshot().GetData().GetCollections(), template.CollectionStoreKey)
	for _, content := range change.GetContent() {
		if update := content.GetStoreSliceUpdate(); update != nil && update.Key == template.CollectionStoreKey && update.GetAdd() != nil {
			ids = append(ids, update.GetAdd().Ids...)
		}
		if set := content.GetStoreKeySet(); set != nil && slices.Equal(set.Path, []string{template.CollectionStoreKey}) {
			ids = append(ids, pbtypes.GetStringListValue(set.Value)...)
		}
	}
	return ids, nil
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
