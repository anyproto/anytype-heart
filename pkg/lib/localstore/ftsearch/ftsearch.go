package ftsearch

/*
#cgo windows,amd64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/windows-amd64 -ltantivy_go -lm -pthread -lws2_32 -lbcrypt -lntdll -luserenv
#cgo darwin,amd64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/darwin-amd64 -ltantivy_go -lm -pthread -ldl
#cgo darwin,arm64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/darwin-arm64 -ltantivy_go -lm -pthread -ldl
#cgo ios,arm64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/ios-arm64 -ltantivy_go -lm -pthread -ldl
#cgo ios,amd64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/ios-amd64 -ltantivy_go -lm -pthread -ldl
#cgo android,arm LDFLAGS:-L${SRCDIR}/../../../../deps/libs/android-arm -ltantivy_go -lm -pthread -ldl
#cgo android,386 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/android-386 -ltantivy_go -lm -pthread -ldl
#cgo android,amd64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/android-amd64 -ltantivy_go -lm -pthread -ldl
#cgo android,arm64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/android-arm64 -ltantivy_go -lm -pthread -ldl
#cgo linux,amd64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/linux-amd64-musl -Wl,--allow-multiple-definition -ltantivy_go -lm
#cgo linux,arm64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/linux-arm64-musl -Wl,--allow-multiple-definition -ltantivy_go -lm
*/
import "C"
import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	tantivy "github.com/anyproto/tantivy-go"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch/tantivycheck"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/text"
)

const (
	CName   = "fts"
	ftsDir  = "fts"
	ftsDir2 = "fts_tantivy"
	// ftsVer names the on-disk index directory. A bump throws every user's
	// index away and rebuilds it from the object store, so it is reserved for
	// changes that make an existing index unusable by design. The schema is
	// pinned by TestSchemaPinned: tantivy refuses to open an index whose
	// stored schema differs from the requested one, and Run recovers from
	// that by quarantining and rebuilding only the affected index (releases
	// up to v0.47.2 wrote a seven-field schema under this same version; from
	// v0.48.0 the four chat message fields are part of it).
	ftsVer   = "16"
	docLimit = 10000

	// defaultSearchLimit is the docs limit applied when the caller passes none
	defaultSearchLimit = 100

	fieldTitle     = "Title"
	fieldTitleZh   = "TitleZh"
	fieldText      = "Text"
	fieldTextZh    = "TextZh"
	fieldSpace     = "SpaceID"
	fieldId        = "Id"
	fieldIdRaw     = "IdRaw"
	fieldAuthor    = "Author"
	fieldOrderId   = "OrderId"
	fieldMessageId = "MessageId"
	fieldTimestamp = "Timestamp"

	score        = "score"
	highlights   = "highlights"
	fragment     = "fragment"
	fieldNameTxt = "field_name"
	tokenizerId  = "SimpleIdTokenizer"
)

// schemaFieldNames is the field order of the schema built in Run; an index
// whose meta.json lists a different set or order cannot be opened by tantivy
var schemaFieldNames = []string{
	fieldId, fieldIdRaw, fieldSpace, fieldTitle, fieldTitleZh, fieldText, fieldTextZh,
	fieldAuthor, fieldOrderId, fieldMessageId, fieldTimestamp,
}

// schemaMismatchMarker is the stable part of tantivy's error when an index
// exists with a different schema ("Schema error: 'An index exists but the
// schema does not match.'"); tantivy-go exposes the error as text only. A
// variable so tests can prove the meta.json fallback works without it.
var schemaMismatchMarker = "schema does not match"

const (
	schemaRecoveryTantivyError    = "tantivy-error"
	schemaRecoveryMetaNames       = "meta-names"
	schemaRecoveryMetaUndecodable = "meta-undecodable"
)

var (
	log                    = logging.Logger("ftsearch")
	ErrAppClosingInitiated = errors.New("app closing initiated")
	ErrIndexInUse          = errors.New("tantivy index is locked by another process")
	ErrSchemaMismatch      = errors.New("tantivy index schema does not match")
)

type FTSearch interface {
	app.ComponentRunnable
	Index(d SearchDoc) (err error)
	NewAutoBatcher() AutoBatcher
	// BatchDeleteObjects deletes ALL documents belonging to the given object ids
	// (doc ids are structured as "objectId/r/relationKey", "objectId/b/blockId",
	// "objectId/m/messageId"; bare-id documents are deleted as well)
	BatchDeleteObjects(ids []string) (err error)
	// Search returns up to limit best-scoring doc matches (limit <= 0 means
	// the default of 100). The limit applies to docs, not objects. Highlight
	// generation costs up to ~130µs per matched doc with large stored text and
	// is pure waste for record-only responses — pass withHighlights=false
	// unless the caller returns highlight fragments to the client.
	Search(spaceId string, query string, limit int, withHighlights bool) (results []*DocumentMatch, err error)
	// SearchChat searches only message documents ("chatId/m/...") so messages
	// don't compete with the rest of the space for the limit. Empty chatId
	// searches messages of all chats; empty spaceId searches all spaces.
	// Non-empty creators restrict to messages authored by those identities
	// (exact match), scoping the candidate budget to their messages.
	SearchChat(spaceId string, chatId string, query string, creators []string, limit int) (results []*DocumentMatch, err error)
	// NamePrefixSearch special prefix case search
	NamePrefixSearch(spaceId string, query string, limit int) (results []*DocumentMatch, err error)
	ListByIdPrefix(prefix string) (ids []string, err error)
	// ListIdsBySpace returns up to limit document ids belonging to the space;
	// callers that need the full set must pass a sufficient limit or
	// delete/iterate and call again (limit <= 0 means one default page of 10k)
	ListIdsBySpace(spaceId string, limit int) (ids []string, err error)
	Iterate(objectId string, fields []string, shouldContinue func(doc *SearchDoc) bool) (err error)
	ListAllObjectIds() (map[string]struct{}, error)
	DocCount() (uint64, error)
	LastDbState() (uint64, error)
	ConsistencyReport() *tantivycheck.ConsistencyReport
}

type SearchDoc struct {
	Id      string
	SpaceId string
	Title   string
	Text    string

	// message specific fields
	Author    string
	OrderId   string
	MessageId string
	Timestamp string
}

type Highlight struct {
	Ranges [][]int `json:"r"`
	Text   string  `json:"t"`
}

type DocumentMatch struct {
	Score     float64
	ID        string
	SpaceId   string
	Fragments map[string]*Highlight
	Fields    map[string]any
}

type ftSearch struct {
	rootPath            string
	ftsPath             string
	builderId           string
	index               *tantivy.TantivyContext
	parserPool          *fastjson.ParserPool
	mu                  sync.Mutex
	blevePath           string
	lang                tantivy.Language
	appClosingInitiated atomic.Bool
	startupReport       *tantivycheck.ConsistencyReport
	// schemaRecovery records which check made Run rebuild an incompatible
	// index ("" when it opened as is); for tests and diagnostics
	schemaRecovery string
}

func (f *ftSearch) LastDbState() (uint64, error) {
	if f.index == nil {
		return 0, fmt.Errorf("index is not initialized")
	}
	lastOpstamp := f.index.CommitOpstamp()
	return lastOpstamp, nil
}

func (f *ftSearch) ProvideStat() any {
	count, _ := f.DocCount()
	return count
}

func (f *ftSearch) StatId() string {
	return "doc_count"
}

func (f *ftSearch) StatType() string {
	return CName
}

func TantivyNew() FTSearch {
	return new(ftSearch)
}

func (f *ftSearch) BatchDeleteObjects(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.appClosingInitiated.Load() {
		return ErrAppClosingInitiated
	}
	start := time.Now()
	defer func() {
		spentMs := time.Since(start).Milliseconds()
		l := log.With("objects", len(ids)).With("total", time.Since(start).Milliseconds())
		if spentMs > 1000 {
			l.Warnf("ft delete took too long")
		} else {
			l.Debugf("ft delete done")
		}
	}()
	// doc ids are full paths ("objectId/r/relationKey", "objectId/b/blockId",
	// "objectId/m/messageId") and deletion is an exact term match on IdRaw, so
	// expand every object id into its doc ids first; listDocIdsForObject
	// returns at most one page, so only objects that returned a full page are
	// re-listed. The iteration cap guards against a non-progressing loop (e.g.
	// deletes committed but the reader persistently failing to reload).
	const maxIterations = 1000
	pending := ids
	for iteration := 0; iteration < maxIterations && len(pending) > 0; iteration++ {
		docIds := make([]string, 0, len(pending))
		var nextPending []string
		for _, id := range pending {
			found, pageFull, err := f.listDocIdsForObject(id)
			if err != nil {
				return fmt.Errorf("list docs by object id: %w", err)
			}
			docIds = append(docIds, found...)
			if pageFull {
				// the object may have more docs beyond the listing page
				nextPending = append(nextPending, id)
			}
		}
		if len(docIds) == 0 {
			return nil
		}
		if err := f.index.DeleteDocuments(fieldIdRaw, docIds...); err != nil {
			return fmt.Errorf("delete object docs: %w", err)
		}
		pending = nextPending
	}
	if len(pending) > 0 {
		return fmt.Errorf("object docs still present after %d delete iterations", maxIterations)
	}

	return nil
}

// listDocIdsForObject returns up to one page (docLimit) of doc ids belonging
// to the object; pageFull reports whether the underlying search page was full,
// i.e. more matches may exist beyond it. It deliberately queries the tokenized
// Id field instead of a TermPrefixQuery on IdRaw: prefix queries expand to at
// most ~50 terms per segment (and deleted terms eat the expansion budget),
// which silently truncates the listing. A term/phrase match has no such cap;
// token-level false positives (another object's doc path containing the same
// token) are filtered out by the exact prefix check.
func (f *ftSearch) listDocIdsForObject(objectId string) (ids []string, pageFull bool, err error) {
	query := tantivy.NewQueryBuilder().
		Query(tantivy.Must, fieldId, objectId, tantivy.PhraseQuery, 1.0).
		Build()
	sCtx := tantivy.NewSearchContextBuilder().
		SetQueryFromJson(&query).
		SetWithHighlights(false).
		AddField(fieldIdRaw, 1.0).
		SetDocsLimit(docLimit).
		Build()

	results, err := f.index.SearchFastFieldJson(sCtx, fieldIdRaw)
	if err != nil {
		return nil, false, fmt.Errorf("search docs for object: %w", err)
	}
	ids = make([]string, 0, len(results.Values))
	prefix := objectId + domain.ObjectPathSeparator
	for _, id := range results.Values {
		if strings.HasPrefix(id, prefix) || id == objectId {
			ids = append(ids, id)
		}
	}
	return ids, len(results.Values) >= docLimit, nil
}

func (f *ftSearch) DeleteObject(objectId string) error {
	return f.BatchDeleteObjects([]string{objectId})
}

func (f *ftSearch) Init(a *app.App) error {
	repoPath := app.MustComponent[wallet.Wallet](a).RepoPath()
	statService, _ := app.GetComponent[debugstat.StatService](a)
	if statService != nil {
		statService.AddProvider(f)
	}
	f.lang = validateLanguage(app.MustComponent[wallet.Wallet](a).FtsPrimaryLang())
	f.rootPath = filepath.Join(repoPath, ftsDir2)
	f.blevePath = filepath.Join(repoPath, ftsDir)
	f.ftsPath = filepath.Join(repoPath, ftsDir2, ftsVer)
	return tantivy.LibInit(false, true, "release")
}

func (f *ftSearch) cleanUpOldIndexes() {
	if strings.HasSuffix(f.rootPath, ftsDir2) {
		dirs, err := os.ReadDir(f.rootPath)
		if err == nil {
			// cleanup old index versions
			for _, dir := range dirs {
				if dir.Name() != ftsVer {
					_ = os.RemoveAll(filepath.Join(f.rootPath, dir.Name()))
				}
			}
		}
	}
}

func (f *ftSearch) Name() (name string) {
	return CName
}

func (f *ftSearch) Run(context.Context) error {
	report, err := tantivycheck.Check(f.ftsPath)
	metaUndecodable := errors.Is(err, tantivycheck.ErrMetaUndecodable)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warnf("tantivy index checking failed: %v", err)
		}
	}
	f.startupReport = &report
	if report.WriterLockPresent || report.MetaLockPresent {
		return ErrIndexInUse
	}
	// Quarantined copies hold nothing worth keeping once this run has
	// decided their fate (rebuilt, or unopenable either way), so sweep them
	// on every exit, including copies an interrupted earlier start left
	// behind. The goroutine may outlive Close; a concurrent account removal
	// deleting the same paths is tolerated by os.RemoveAll.
	defer func() { go f.removeQuarantinedIndexes() }()
	var quarantinePath string
	if len(report.MissingSegments) > 0 || len(report.MissingDelFiles) > 0 {
		quarantinePath, err = f.quarantineCorruptIndex()
		if err != nil {
			return err
		}
		f.startupReport = &tantivycheck.ConsistencyReport{Rebuilt: true, ReportTime: time.Now()}
	}
	if !report.IsOk() {
		var gcErr error
		if quarantinePath == "" && (len(report.ExtraDelFiles) > 0 || len(report.ExtraSegments) > 0) {
			gcErr = report.GCExtraFiles()
		}
		log.With("missingSegments", len(report.MissingSegments)).
			With("missingDelFiles", len(report.MissingDelFiles)).
			With("extraSegments", len(report.ExtraSegments)).
			With("extraDelFiles", len(report.ExtraDelFiles)).
			With("writerLockPresent", report.WriterLockPresent).
			With("metaLockPresent", report.MetaLockPresent).
			With("totalSegmentsInMeta", report.TotalSegmentsInMeta).
			With("uniqueSegmentPrefixesOnDisk", report.UniqueSegmentPrefixesOnDisk).
			With("oldestSegmentModTime", report.OldestSegmentModTime.Unix()).
			With("newestSegmentModTime", report.NewestSegmentModTime.Unix()).
			With("metaJsonModTime", report.MetaJsonModTime.Unix()).
			With("gcErr", gcErr).
			Warnf("tantivy index was inconsistent during startup")
	}

	builder, err := tantivy.NewSchemaBuilder()
	if err != nil {
		return err
	}

	err = builder.AddTextField(
		fieldId, // 0
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tokenizerId,
	)
	if err != nil {
		return fmt.Errorf("add id field: %w", err)
	}

	err = builder.AddTextField(
		fieldIdRaw, // 1
		true,
		true,
		true,
		tantivy.IndexRecordOptionBasic,
		tantivy.TokenizerRaw,
	)
	if err != nil {
		return fmt.Errorf("add id raw field: %w", err)
	}

	err = builder.AddTextField(
		fieldSpace, // 2
		true,
		false,
		true,
		tantivy.IndexRecordOptionBasic,
		tantivy.TokenizerRaw,
	)
	if err != nil {
		return fmt.Errorf("add space id field: %w", err)
	}

	err = builder.AddTextField(
		fieldTitle, // 3
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tantivy.TokenizerSimple,
	)
	if err != nil {
		return fmt.Errorf("add title field: %w", err)
	}

	err = builder.AddTextField(
		fieldTitleZh, // 4
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tantivy.TokenizerJieba,
	)
	if err != nil {
		return fmt.Errorf("add Chinese title field: %w", err)
	}

	err = builder.AddTextField(
		fieldText, // 5
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tantivy.TokenizerSimple,
	)
	if err != nil {
		return fmt.Errorf("add text field: %w", err)
	}

	err = builder.AddTextField(
		fieldTextZh, // 6
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tantivy.TokenizerJieba,
	)
	if err != nil {
		return fmt.Errorf("add Chinese text field: %w", err)
	}

	err = builder.AddTextField(
		fieldAuthor, // 7
		true,
		false,
		true,
		tantivy.IndexRecordOptionBasic,
		tantivy.TokenizerRaw,
	)
	if err != nil {
		return fmt.Errorf("add author field: %w", err)
	}

	err = builder.AddTextField(
		fieldOrderId, // 8
		true,
		false,
		true,
		tantivy.IndexRecordOptionBasic,
		tantivy.TokenizerRaw,
	)
	if err != nil {
		return fmt.Errorf("add orderId field: %w", err)
	}

	err = builder.AddTextField(
		fieldMessageId, // 9
		true,
		false,
		true,
		tantivy.IndexRecordOptionBasic,
		tantivy.TokenizerRaw,
	)
	if err != nil {
		return fmt.Errorf("add message Id field: %w", err)
	}

	err = builder.AddTextField(
		fieldTimestamp, // 10
		true,
		false,
		true,
		tantivy.IndexRecordOptionBasic,
		tantivy.TokenizerRaw,
	)
	if err != nil {
		return fmt.Errorf("add message timestamp field: %w", err)
	}

	schema, err := builder.BuildSchema()
	if err != nil {
		return err
	}
	index, err := f.tryToBuildSchema(schema)
	if err != nil && quarantinePath == "" {
		// An index written with a different schema (e.g. by a release with
		// fewer fields) or whose meta.json is no longer valid JSON cannot be
		// opened by tantivy. It is derived data: quarantine it and start from
		// an empty one, the indexer rebuilds it from the object store (ftInit
		// sees an empty index). The rebuild is only ever triggered by a
		// failed open, so a healthy index can never be thrown away; lock,
		// permission and I/O errors are not recovered.
		f.schemaRecovery = unopenableIndexReason(err, report.SchemaFieldNames, metaUndecodable)
		if f.schemaRecovery != "" {
			log.With("recovery", f.schemaRecovery).With("onDisk", report.SchemaFieldNames).
				Warnf("tantivy index schema mismatch, rebuilding: %v", err)
			if _, err = f.quarantineCorruptIndex(); err != nil {
				return err
			}
			f.startupReport = &tantivycheck.ConsistencyReport{Rebuilt: true, ReportTime: time.Now()}
			index, err = f.tryToBuildSchema(schema)
		}
	}
	if err != nil {
		return fmt.Errorf("open tantivy index: %w", err)
	}
	f.index = index
	f.parserPool = &fastjson.ParserPool{}

	f.cleanupBleve()
	f.cleanUpOldIndexes()

	err = index.RegisterTextAnalyzerSimple(tantivy.TokenizerSimple, 40, f.lang)
	if err != nil {
		return err
	}

	err = index.RegisterTextAnalyzerJieba(tantivy.TokenizerJieba, 40)
	if err != nil {
		return err
	}

	err = index.RegisterTextAnalyzerSimple(tokenizerId, 1000, tantivy.English)
	if err != nil {
		return err
	}

	err = index.RegisterTextAnalyzerNgram(tantivy.TokenizerNgram, 1, 5, false)
	if err != nil {
		return err
	}

	err = index.RegisterTextAnalyzerRaw(tantivy.TokenizerRaw)
	if err != nil {
		return err
	}

	return nil
}

const quarantineSuffix = ".corrupt-"

// quarantineCorruptIndex moves the whole index root aside so a fresh index
// can be created in its place; nothing is deleted until the replacement
// index has been opened (see removeQuarantinedIndexes)
func (f *ftSearch) quarantineCorruptIndex() (string, error) {
	if !strings.HasSuffix(f.rootPath, ftsDir2) {
		return "", fmt.Errorf("refusing to quarantine unexpected path %s", f.rootPath)
	}
	quarantinePath := fmt.Sprintf("%s%s%d", f.rootPath, quarantineSuffix, time.Now().UnixNano())
	if err := os.Rename(f.rootPath, quarantinePath); err != nil {
		return "", fmt.Errorf("quarantine corrupt tantivy index: %w", err)
	}
	log.Warnf("quarantined corrupt tantivy index at %s", quarantinePath)
	return quarantinePath, nil
}

// removeQuarantinedIndexes deletes every quarantined copy next to the index
// root, including ones an earlier start left behind (killed during removal,
// or a failed rebuild). A plain directory listing is used rather than a glob
// so metacharacters in the account path cannot widen the match.
func (f *ftSearch) removeQuarantinedIndexes() {
	if !strings.HasSuffix(f.rootPath, ftsDir2) {
		return
	}
	parent := filepath.Dir(f.rootPath)
	entries, err := os.ReadDir(parent)
	if err != nil {
		log.Warnf("list quarantined tantivy indexes: %v", err)
		return
	}
	prefix := filepath.Base(f.rootPath) + quarantineSuffix
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		path := filepath.Join(parent, entry.Name())
		if removeErr := os.RemoveAll(path); removeErr != nil {
			log.Warnf("failed to remove quarantined tantivy index %s: %v", path, removeErr)
		}
	}
}

func (f *ftSearch) tryToBuildSchema(schema *tantivy.Schema) (*tantivy.TantivyContext, error) {
	index, err := tantivy.NewTantivyContextWithSchema(f.ftsPath, schema)
	if err != nil && isSchemaMismatchError(err) {
		return nil, fmt.Errorf("%w: %w", ErrSchemaMismatch, err)
	}
	return index, err
}

// isSchemaMismatchError reports whether tantivy refused to open an existing
// index because its stored schema differs from the requested one
func isSchemaMismatchError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), schemaMismatchMarker)
}

// unopenableIndexReason classifies a failed open as one the index can be
// rebuilt from: tantivy's own schema verdict first, then the field names
// recorded in meta.json as a backstop should the error text ever change,
// then a meta.json that is not valid JSON (detected locally, never from
// tantivy's generic corruption/I/O text). Empty means the failure must
// propagate.
func unopenableIndexReason(openErr error, onDiskFieldNames []string, metaUndecodable bool) string {
	if errors.Is(openErr, ErrSchemaMismatch) {
		return schemaRecoveryTantivyError
	}
	if len(onDiskFieldNames) > 0 && !slices.Equal(onDiskFieldNames, schemaFieldNames) {
		return schemaRecoveryMetaNames
	}
	if metaUndecodable {
		return schemaRecoveryMetaUndecodable
	}
	return ""
}

func (f *ftSearch) Index(doc SearchDoc) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.appClosingInitiated.Load() {
		return ErrAppClosingInitiated
	}
	metrics.ObjectFTUpdatedCounter.Inc()
	tantivyDoc, err := f.convertDoc(doc)
	if err != nil {
		return err
	}

	_, res := f.index.AddAndConsumeDocumentsWithOpstamp(tantivyDoc)
	return res
}

func (f *ftSearch) convertDoc(doc SearchDoc) (*tantivy.Document, error) {
	document := tantivy.NewDocument()
	err := document.AddFields(doc.Id, f.index, fieldId, fieldIdRaw)
	if err != nil {
		return nil, err
	}
	err = document.AddField(doc.SpaceId, f.index, fieldSpace)
	if err != nil {
		return nil, err
	}
	err = document.AddFields(doc.Title, f.index, fieldTitle, fieldTitleZh)
	if err != nil {
		return nil, err
	}
	err = document.AddFields(doc.Text, f.index, fieldText, fieldTextZh)
	if err != nil {
		return nil, err
	}
	err = document.AddFields(doc.Author, f.index, fieldAuthor)
	if err != nil {
		return nil, err
	}
	err = document.AddFields(doc.OrderId, f.index, fieldOrderId)
	if err != nil {
		return nil, err
	}
	err = document.AddFields(doc.MessageId, f.index, fieldMessageId)
	if err != nil {
		return nil, err
	}
	err = document.AddFields(doc.Timestamp, f.index, fieldTimestamp)
	if err != nil {
		return nil, err
	}
	return document, nil
}

func (f *ftSearch) NamePrefixSearch(spaceId, query string, limit int) ([]*DocumentMatch, error) {
	// the prefix search consumers don't use text fragments, so skip the
	// expensive highlight generation entirely
	return f.performSearch(spaceId, query, limit, false, f.buildObjectQuery)
}

func (f *ftSearch) Search(spaceId, query string, limit int, withHighlights bool) ([]*DocumentMatch, error) {
	return f.performSearch(spaceId, query, limit, withHighlights, f.buildDetailedQuery)
}

func (f *ftSearch) SearchChat(spaceId, chatId, query string, creators []string, limit int) ([]*DocumentMatch, error) {
	return f.performSearch(spaceId, query, limit, true, func(qb *tantivy.QueryBuilder, query string) {
		// restrict to the given authors: the Author field is raw-indexed, so
		// term queries match the exact identity string. Scoping here rather
		// than post-filtering keeps the candidate budget on the requested
		// authors' messages. Boost 0 keeps the clause out of BM25 scoring.
		authors := make([]string, 0, len(creators))
		for _, creator := range creators {
			// an empty term would compile to a match-nothing clause
			if creator != "" {
				authors = append(authors, creator)
			}
		}
		if len(authors) == 1 {
			qb.Query(tantivy.Must, fieldAuthor, authors[0], tantivy.TermQuery, 0.0)
		} else if len(authors) > 1 {
			nested := qb.NestedBuilder()
			for _, author := range authors {
				nested.Query(tantivy.Should, fieldAuthor, author, tantivy.TermQuery, 0.0)
			}
			qb.BooleanQuery(tantivy.Must, nested, 0.0)
		}
		if chatId != "" {
			// restrict to the chat's message docs ("chatId/m/msgId") via a phrase
			// match on the tokenized id field. A prefix query on IdRaw would
			// silently truncate: prefix expansion is capped at ~50 terms per
			// segment. Boost 0 keeps the clause out of BM25 scoring.
			qb.Query(tantivy.Must, fieldId, chatId+"/m", tantivy.PhraseQuery, 0.0)
		} else {
			// all message docs: every "chatId/m/msgId" id contains the token "m"
			// (the id tokenizer has no stopword filter) while object/relation/block
			// doc ids tokenize to longer alphanumeric tokens. An exists-query on
			// the MessageId field is not expressible in tantivy-go, so the token is
			// the message-doc marker; callers drop the rare false positive via
			// path.HasMessage(). Boost 0 keeps the clause out of BM25 scoring.
			//
			// Index efficiency: the Must intersection is driven by the rarer text
			// postings and only seeks into the "m" posting list via block skip
			// lists, so cost scales with text matches, not with total messages.
			// The per-chat phrase clause above walks the same "m" postings (plus
			// position checks); a dedicated doc-type field would have an identical
			// posting-list profile while requiring an index-version bump + full
			// reindex.
			qb.Query(tantivy.Must, fieldId, "m", tantivy.TermQuery, 0.0)
		}
		f.buildDetailedQuery(qb, query)
	})
}

func (f *ftSearch) performSearch(spaceId, query string, limit int, withHighlights bool, buildQueryFunc func(*tantivy.QueryBuilder, string)) ([]*DocumentMatch, error) {
	query = prepareQuery(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	qb := tantivy.NewQueryBuilder()
	if len(spaceId) != 0 {
		// boost 0 keeps the scope clause out of BM25 scoring: the space term's
		// IDF depends on the space's share of the index, so at boost 1 it
		// added a per-space additive bias (larger for smaller spaces) that
		// made scores incomparable across spaces. Within one space the term
		// contributed a constant, so per-space ranking is unchanged.
		qb.Query(tantivy.Must, fieldSpace, spaceId, tantivy.TermQuery, 0.0)
	}

	buildQueryFunc(qb, query)

	finalQuery := qb.Build()
	sCtx := tantivy.NewSearchContextBuilder().
		SetQueryFromJson(&finalQuery).
		SetDocsLimit(uintptr(limit)).
		SetWithHighlights(withHighlights).
		Build()

	result, err := f.index.SearchJson(sCtx)
	if err != nil {
		return nil, wrapError(err)
	}

	p := f.parserPool.Get()
	defer f.parserPool.Put(p)

	return tantivy.GetSearchResults(
		result,
		f.index,
		func(json string) (*DocumentMatch, error) {
			return parseSearchResult(json, p)
		},
		fieldId,
		fieldSpace,
	)
}

func (f *ftSearch) buildObjectQuery(qb *tantivy.QueryBuilder, query string) {
	qb.BooleanQuery(tantivy.Must, qb.NestedBuilder().
		Query(tantivy.Should, fieldId, bundle.RelationKeyName.String(), tantivy.TermQuery, 1.0).
		// snippets are indexed only for notes which don't have a name, we should do a prefix search there as well
		Query(tantivy.Should, fieldId, bundle.RelationKeySnippet.String(), tantivy.TermQuery, 1.0).
		Query(tantivy.Should, fieldId, bundle.RelationKeyPluralName.String(), tantivy.TermQuery, 1.0),
		1.0,
	)

	if containsChineseCharacters(query) {
		qb.BooleanQuery(tantivy.Must, qb.NestedBuilder().
			Query(tantivy.Should, fieldTitleZh, query, tantivy.PhrasePrefixQuery, 1.0).
			Query(tantivy.Should, fieldTextZh, query, tantivy.PhrasePrefixQuery, 1.0),
			1.0,
		)
	} else {
		qb.BooleanQuery(tantivy.Must, qb.NestedBuilder().
			Query(tantivy.Should, fieldTitle, query, tantivy.PhrasePrefixQuery, 1.0).
			Query(tantivy.Should, fieldText, query, tantivy.PhrasePrefixQuery, 1.0),
			1.0,
		)
	}
}

func (f *ftSearch) buildDetailedQuery(qb *tantivy.QueryBuilder, query string) {
	if containsChineseCharacters(query) {
		qb.BooleanQuery(tantivy.Must, qb.NestedBuilder().
			Query(tantivy.Should, fieldTitleZh, query, tantivy.PhrasePrefixQuery, 20.0).
			Query(tantivy.Should, fieldTitleZh, query, tantivy.PhraseQuery, 20.0).
			Query(tantivy.Should, fieldTitleZh, query, tantivy.EveryTermQuery, 0.75).
			Query(tantivy.Should, fieldTitleZh, query, tantivy.OneOfTermQuery, 0.5).
			Query(tantivy.Should, fieldTextZh, query, tantivy.PhrasePrefixQuery, 1.0).
			Query(tantivy.Should, fieldTextZh, query, tantivy.PhraseQuery, 1.0).
			Query(tantivy.Should, fieldTextZh, query, tantivy.EveryTermQuery, 0.5).
			Query(tantivy.Should, fieldTextZh, query, tantivy.OneOfTermQuery, 0.25),
			1.0,
		)
	} else {
		qb.BooleanQuery(tantivy.Must, qb.NestedBuilder().
			Query(tantivy.Should, fieldTitle, query, tantivy.PhrasePrefixQuery, 20.0).
			Query(tantivy.Should, fieldTitle, query, tantivy.PhraseQuery, 20.0).
			Query(tantivy.Should, fieldTitle, query, tantivy.EveryTermQuery, 0.75).
			Query(tantivy.Should, fieldTitle, query, tantivy.OneOfTermQuery, 0.5).
			Query(tantivy.Should, fieldText, query, tantivy.PhrasePrefixQuery, 1.0).
			Query(tantivy.Should, fieldText, query, tantivy.PhraseQuery, 1.0).
			Query(tantivy.Should, fieldText, query, tantivy.EveryTermQuery, 0.5).
			Query(tantivy.Should, fieldText, query, tantivy.OneOfTermQuery, 0.25),
			1.0,
		)
	}
}

func parseSearchResult(json string, parser *fastjson.Parser) (*DocumentMatch, error) {
	value, err := parser.Parse(json)
	if err != nil {
		return nil, wrapError(err)
	}

	highlights := value.GetArray(highlights)
	fragments := map[string]*Highlight{}

	for _, val := range highlights {
		object := val.GetObject()
		fieldName := string(object.Get(fieldNameTxt).GetStringBytes())

		if fieldName == fieldTitle || fieldName == fieldTitleZh {
			fragments = map[string]*Highlight{}
			break
		}

		if fieldName == fieldText || fieldName == fieldTextZh {
			extractHighlight(object, fragments, fieldName)
		}
	}

	if len(fragments) == 2 {
		// Remove Chinese highlights if non-Chinese highlights are present
		delete(fragments, fieldTextZh)
	}

	return &DocumentMatch{
		Score: value.GetFloat64(score),
		ID:    string(value.GetStringBytes(fieldId)),
		// the doc's stored space field: lets cross-space searches attribute
		// hits without extra lookups
		SpaceId:   string(value.GetStringBytes(fieldSpace)),
		Fragments: fragments,
	}, nil
}

func containsChineseCharacters(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func extractHighlight(object *fastjson.Object, fragments map[string]*Highlight, fieldName string) {
	highlightObj := object.Get(fragment)
	if highlightObj == nil {
		return
	}
	highlight := Highlight{}
	fragments[fieldName] = &highlight
	rangesArray := highlightObj.GetArray("r")
	for _, innerArray := range rangesArray {
		rangeValues := innerArray.GetArray()
		if len(rangeValues) == 2 {
			start := rangeValues[0].GetInt()
			end := rangeValues[1].GetInt()
			highlight.Ranges = append(highlight.Ranges, []int{start, end})
			highlight.Text = string(highlightObj.GetStringBytes("t"))
		}
	}
}

func wrapError(err error) error {
	errStr := err.Error()
	if strings.Contains(errStr, "Syntax Error:") {
		return fmt.Errorf("invalid query")
	}
	return err
}

func (f *ftSearch) Delete(id string) error {
	return f.BatchDeleteObjects([]string{id})
}

func (f *ftSearch) DocCount() (uint64, error) {
	return f.index.NumDocs()
}

func (f *ftSearch) Close(ctx context.Context) error {
	if f.index != nil {
		err := f.index.Close()
		if err != nil {
			log.Errorf("failed to close tantivy index: %v", err)
		}
	}
	return nil
}

func (f *ftSearch) ConsistencyReport() *tantivycheck.ConsistencyReport {
	return f.startupReport
}

func (f *ftSearch) cleanupBleve() {
	_ = os.RemoveAll(f.blevePath)
}

func (f *ftSearch) StateChange(state int) {
	if state == int(domain.CompStateAppClosingInitiated) {
		f.appClosingInitiated.Store(true)
	}
}

func prepareQuery(query string) string {
	query = text.Truncate(query, 100, "")
	query = strings.ToLower(query)
	query = strings.TrimSpace(query)
	return query
}

func validateLanguage(lang string) tantivy.Language {
	tantivyLang := tantivy.Language(lang)
	switch tantivyLang {
	case tantivy.Arabic, tantivy.Armenian, tantivy.Basque, tantivy.Catalan, tantivy.Danish, tantivy.Dutch, tantivy.English,
		tantivy.Estonian, tantivy.Finnish, tantivy.French, tantivy.German, tantivy.Greek, tantivy.Hindi, tantivy.Hungarian,
		tantivy.Indonesian, tantivy.Irish, tantivy.Italian, tantivy.Lithuanian, tantivy.Nepali, tantivy.Norwegian,
		tantivy.Portuguese, tantivy.Romanian, tantivy.Russian, tantivy.Serbian, tantivy.Spanish, tantivy.Swedish,
		tantivy.Tamil, tantivy.Turkish, tantivy.Yiddish:
		return tantivyLang
	default:
		return tantivy.English
	}
}
