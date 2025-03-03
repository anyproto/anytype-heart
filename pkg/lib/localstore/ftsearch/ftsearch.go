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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/anyproto/any-sync/app"
	tantivy "github.com/anyproto/tantivy-go"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/text"
)

const (
	CName    = "fts"
	ftsDir   = "fts"
	ftsDir2  = "fts_tantivy"
	ftsVer   = "11"
	docLimit = 10000

	fieldTitle   = "Title"
	fieldTitleZh = "TitleZh"
	fieldText    = "Text"
	fieldTextZh  = "TextZh"
	fieldSpace   = "SpaceID"
	fieldId      = "Id"
	fieldIdRaw   = "IdRaw"
	score        = "score"
	highlights   = "highlights"
	fragment     = "fragment"
	fieldNameTxt = "field_name"
	tokenizerId  = "SimpleIdTokenizer"
)

var log = logging.Logger("ftsearch")

type FTSearch interface {
	app.ComponentRunnable
	Index(d SearchDoc) (err error)
	NewAutoBatcher() AutoBatcher
	BatchIndex(ctx context.Context, docs []SearchDoc, deletedDocs []string) (err error)
	BatchDeleteObjects(ids []string) (err error)
	Search(spaceId string, query string) (results []*DocumentMatch, err error)
	// NamePrefixSearch special prefix case search
	NamePrefixSearch(spaceId string, query string) (results []*DocumentMatch, err error)
	Iterate(objectId string, fields []string, shouldContinue func(doc *SearchDoc) bool) (err error)
	DeleteObject(id string) error
	DocCount() (uint64, error)
}

type SearchDoc struct {
	Id      string
	SpaceId string
	Title   string
	Text    string
}

type Highlight struct {
	Ranges [][]int `json:"r"`
	Text   string  `json:"t"`
}

type DocumentMatch struct {
	Score     float64
	ID        string
	Fragments map[string]*Highlight
	Fields    map[string]any
}

type ftSearchTantivy struct {
	rootPath   string
	ftsPath    string
	builderId  string
	index      *tantivy.TantivyContext
	parserPool *fastjson.ParserPool
	mu         sync.Mutex
	blevePath  string
	lang       tantivy.Language
}

func TantivyNew() FTSearch {
	return new(ftSearchTantivy)
}

func (f *ftSearchTantivy) BatchDeleteObjects(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
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
	err := f.index.DeleteDocuments(fieldIdRaw, ids...)
	if err != nil {
		return err
	}

	return nil
}

func (f *ftSearchTantivy) DeleteObject(objectId string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.index.DeleteDocuments(fieldIdRaw, objectId)
}

func (f *ftSearchTantivy) Init(a *app.App) error {
	repoPath := app.MustComponent[wallet.Wallet](a).RepoPath()
	f.lang = validateLanguage(app.MustComponent[wallet.Wallet](a).FtsPrimaryLang())
	f.rootPath = filepath.Join(repoPath, ftsDir2)
	f.blevePath = filepath.Join(repoPath, ftsDir)
	f.ftsPath = filepath.Join(repoPath, ftsDir2, ftsVer)
	return tantivy.LibInit(false, true, "release")
}

func (f *ftSearchTantivy) cleanUpOldIndexes() {
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

func (f *ftSearchTantivy) Name() (name string) {
	return CName
}

func (f *ftSearchTantivy) Run(context.Context) error {
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

	err = builder.AddTextField(
		fieldIdRaw, // 1
		true,
		true,
		true,
		tantivy.IndexRecordOptionBasic,
		tantivy.TokenizerRaw,
	)

	err = builder.AddTextField(
		fieldSpace, // 2
		true,
		false,
		true,
		tantivy.IndexRecordOptionBasic,
		tantivy.TokenizerRaw,
	)

	err = builder.AddTextField(
		fieldTitle, // 3
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tantivy.TokenizerSimple,
	)

	err = builder.AddTextField(
		fieldTitleZh, // 4
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tantivy.TokenizerJieba,
	)

	err = builder.AddTextField(
		fieldText, // 5
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tantivy.TokenizerSimple,
	)

	err = builder.AddTextField(
		fieldTextZh, // 6
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tantivy.TokenizerJieba,
	)

	schema, err := builder.BuildSchema()
	if err != nil {
		return err
	}
	index, err := f.tryToBuildSchema(schema)
	if err != nil {
		return err
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

func (f *ftSearchTantivy) tryToBuildSchema(schema *tantivy.Schema) (*tantivy.TantivyContext, error) {
	index, err := tantivy.NewTantivyContextWithSchema(f.ftsPath, schema)
	if err != nil {
		log.Warnf("recovering from error: %v", err)
		if strings.HasSuffix(f.rootPath, ftsDir2) {
			_ = os.RemoveAll(f.rootPath)
		}
		return tantivy.NewTantivyContextWithSchema(f.ftsPath, schema)
	}
	return index, err
}

func (f *ftSearchTantivy) Index(doc SearchDoc) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	metrics.ObjectFTUpdatedCounter.Inc()
	tantivyDoc, err := f.convertDoc(doc)
	if err != nil {
		return err
	}

	res := f.index.AddAndConsumeDocuments(tantivyDoc)
	return res
}

func (f *ftSearchTantivy) convertDoc(doc SearchDoc) (*tantivy.Document, error) {
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
	return document, nil
}

func (f *ftSearchTantivy) BatchIndex(ctx context.Context, docs []SearchDoc, deletedDocs []string) (err error) {
	if len(docs) == 0 {
		return nil
	}
	metrics.ObjectFTUpdatedCounter.Add(float64(len(docs)))
	start := time.Now()
	defer func() {
		spentMs := time.Since(start).Milliseconds()
		l := log.With("objects", len(docs)).With("total", time.Since(start).Milliseconds())
		if spentMs > 1000 {
			l.Warnf("ft index took too long")
		} else {
			l.Debugf("ft index done")
		}
	}()
	f.mu.Lock()
	err = f.index.DeleteDocuments(fieldIdRaw, deletedDocs...)
	f.mu.Unlock()
	if err != nil {
		return err
	}
	tantivyDocs := make([]*tantivy.Document, 0, len(docs))
	for _, doc := range docs {
		tantivyDoc, err := f.convertDoc(doc)
		if err != nil {
			return err
		}
		tantivyDocs = append(tantivyDocs, tantivyDoc)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.index.AddAndConsumeDocuments(tantivyDocs...)
}

func (f *ftSearchTantivy) NamePrefixSearch(spaceId, query string) ([]*DocumentMatch, error) {
	return f.performSearch(spaceId, query, f.buildObjectQuery)
}

func (f *ftSearchTantivy) Search(spaceId, query string) ([]*DocumentMatch, error) {
	return f.performSearch(spaceId, query, f.buildDetailedQuery)
}

func (f *ftSearchTantivy) performSearch(spaceId, query string, buildQueryFunc func(*tantivy.QueryBuilder, string)) ([]*DocumentMatch, error) {
	query = prepareQuery(query)
	if query == "" {
		return nil, nil
	}

	qb := tantivy.NewQueryBuilder()
	if len(spaceId) != 0 {
		qb.Query(tantivy.Must, fieldSpace, spaceId, tantivy.TermQuery, 1.0)
	}

	buildQueryFunc(qb, query)

	finalQuery := qb.Build()
	sCtx := tantivy.NewSearchContextBuilder().
		SetQueryFromJson(&finalQuery).
		SetDocsLimit(100).
		SetWithHighlights(true).
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
	)
}

func (f *ftSearchTantivy) buildObjectQuery(qb *tantivy.QueryBuilder, query string) {
	qb.BooleanQuery(tantivy.Must, qb.NestedBuilder().
		Query(tantivy.Should, fieldId, bundle.RelationKeyName.String(), tantivy.TermQuery, 1.0).
		// snippets are indexed only for notes which don't have a name, we should do a prefix search there as well
		Query(tantivy.Should, fieldId, bundle.RelationKeySnippet.String(), tantivy.TermQuery, 1.0),
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

func (f *ftSearchTantivy) buildDetailedQuery(qb *tantivy.QueryBuilder, query string) {
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
		Score:     value.GetFloat64(score),
		ID:        string(value.GetStringBytes(fieldId)),
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

func (f *ftSearchTantivy) Delete(id string) error {
	return f.BatchDeleteObjects([]string{id})
}

func (f *ftSearchTantivy) DocCount() (uint64, error) {
	return f.index.NumDocs()
}

func (f *ftSearchTantivy) Close(ctx context.Context) error {
	if f.index != nil {
		f.index.Free()
		f.index = nil
	}
	return nil
}

func (f *ftSearchTantivy) cleanupBleve() {
	_ = os.RemoveAll(f.blevePath)
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
