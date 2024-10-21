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
*/
import "C"
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	tantivy "github.com/anyproto/tantivy-go"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/samber/lo"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/util/text"
)

func TantivyNew() FTSearch {
	return new(ftSearchTantivy)
}

var specialChars = map[rune]struct{}{
	'+': {}, '^': {}, '`': {}, ':': {}, '{': {},
	'}': {}, '"': {}, '[': {}, ']': {}, '(': {},
	')': {}, '~': {}, '!': {}, '\\': {}, '*': {},
}

type ftSearchTantivy struct {
	rootPath   string
	ftsPath    string
	builderId  string
	index      *tantivy.TantivyContext
	schema     *tantivy.Schema
	parserPool *fastjson.ParserPool
}

func (f *ftSearchTantivy) BatchDeleteObjects(ids []string) error {
	if len(ids) == 0 {
		return nil
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
	err := f.index.DeleteDocuments(fieldIdRaw, ids...)
	if err != nil {
		return err
	}

	return nil
}

func (f *ftSearchTantivy) DeleteObject(objectId string) error {
	return f.index.DeleteDocuments(fieldIdRaw, objectId)
}

var ftsDir2 = "fts_tantivy"

func (f *ftSearchTantivy) Init(a *app.App) error {
	repoPath := app.MustComponent[wallet.Wallet](a).RepoPath()
	f.rootPath = filepath.Join(repoPath, ftsDir2)
	f.ftsPath = filepath.Join(repoPath, ftsDir2, ftsVer)
	return tantivy.LibInit("release")
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
		fieldId,
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tokenizerId,
	)

	err = builder.AddTextField(
		fieldIdRaw,
		true,
		true,
		true,
		tantivy.IndexRecordOptionBasic,
		tantivy.TokenizerRaw,
	)

	err = builder.AddTextField(
		fieldSpace,
		true,
		false,
		true,
		tantivy.IndexRecordOptionBasic,
		tantivy.TokenizerRaw,
	)

	err = builder.AddTextField(
		fieldTitle,
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tantivy.TokenizerNgram,
	)

	err = builder.AddTextField(
		fieldText,
		true,
		true,
		false,
		tantivy.IndexRecordOptionWithFreqsAndPositions,
		tantivy.TokenizerSimple,
	)

	schema, err := builder.BuildSchema()
	if err != nil {
		return err
	}
	index, err := f.tryToBuildSchema(schema)
	if err != nil {
		return err
	}
	f.schema = schema
	f.index = index
	f.parserPool = &fastjson.ParserPool{}

	f.cleanupBleve()
	f.cleanUpOldIndexes()

	err = index.RegisterTextAnalyzerSimple(tantivy.TokenizerSimple, 40, tantivy.English)
	if err != nil {
		return err
	}

	err = index.RegisterTextAnalyzerSimple(tokenizerId, 1000, tantivy.English)
	if err != nil {
		return err
	}

	err = index.RegisterTextAnalyzerNgram(tantivy.TokenizerNgram, 3, 5, false)
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
		f.recover()
		return tantivy.NewTantivyContextWithSchema(f.ftsPath, schema)
	}
	return index, err
}

func (f *ftSearchTantivy) Index(doc SearchDoc) error {
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
	err := document.AddField(fieldId, doc.Id, f.index)
	if err != nil {
		return nil, err
	}
	err = document.AddField(fieldIdRaw, doc.Id, f.index)
	if err != nil {
		return nil, err
	}
	err = document.AddField(fieldSpace, doc.SpaceID, f.index)
	if err != nil {
		return nil, err
	}
	err = document.AddField(fieldTitle, doc.Title, f.index)
	if err != nil {
		return nil, err
	}
	err = document.AddField(fieldText, doc.Text, f.index)
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
	err = f.index.DeleteDocuments(fieldIdRaw, deletedDocs...)
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
	return f.index.AddAndConsumeDocuments(tantivyDocs...)
}

func (f *ftSearchTantivy) Search(spaceIds []string, highlightFormatter HighlightFormatter, query string) (results search.DocumentMatchCollection, err error) {
	spaceIdsQuery := getSpaceIdsQuery(spaceIds)
	query = prepareQuery(query)
	if query == "" {
		return nil, nil
	}
	if spaceIdsQuery != "" {
		query = fmt.Sprintf("%s AND %s", spaceIdsQuery, query)
	}
	result, err := f.index.Search(query, 100, true, fieldId, fieldSpace, fieldTitle, fieldText)
	if err != nil {
		return nil, wrapError(err)
	}
	p := f.parserPool.Get()
	defer f.parserPool.Put(p)

	return tantivy.GetSearchResults(
		result,
		f.schema,
		func(json string) (*search.DocumentMatch, error) {
			value, err := p.Parse(json)
			if err != nil {
				return nil, err
			}
			highlights := value.GetArray(highlights)

			fragments := map[string][]string{}
			for _, val := range highlights {
				object := val.GetObject()
				fieldName := string(object.Get("field_name").GetStringBytes())
				if fieldName == fieldTitle {
					// fragments[fieldTitle] = append(fragments[fieldTitle], string(object.Get("fragment").MarshalTo(nil)))
				} else if fieldName == fieldText {
					fragments[fieldText] = append(fragments[fieldText], string(object.Get("fragment").MarshalTo(nil)))
				}
			}

			return &search.DocumentMatch{
				Score:     value.GetFloat64(score),
				ID:        string(value.GetStringBytes(fieldId)),
				Fragments: fragments,
			}, nil
		},
		fieldId,
	)
}

func wrapError(err error) error {
	errStr := err.Error()
	if strings.Contains(errStr, "Syntax Error:") {
		return fmt.Errorf("invalid query")
	}
	return err
}

func getSpaceIdsQuery(ids []string) string {
	ids = lo.Filter(ids, func(item string, index int) bool { return item != "" })
	if len(ids) == 0 || lo.EveryBy(ids, func(id string) bool { return id == "" }) {
		return ""
	}
	var builder strings.Builder
	var sep string

	builder.WriteString("(")
	for _, id := range ids {
		builder.WriteString(sep)
		builder.WriteString(fieldSpace)
		builder.WriteString(":")
		builder.WriteString(id)
		sep = " OR "
	}
	builder.WriteString(")")
	return builder.String()
}

func (f *ftSearchTantivy) Delete(id string) error {
	return f.BatchDeleteObjects([]string{id})
}

func (f *ftSearchTantivy) DocCount() (uint64, error) {
	return f.index.NumDocs()
}

func (f *ftSearchTantivy) Close(ctx context.Context) error {
	f.schema = nil
	if f.index != nil {
		f.index.Free()
		f.index = nil
		f.schema = nil
	}
	return nil
}

func (f *ftSearchTantivy) cleanupBleve() {
	_ = os.RemoveAll(filepath.Join(f.rootPath, ftsDir))
}

func (f *ftSearchTantivy) recover() {
	if strings.HasSuffix(f.rootPath, ftsDir2) {
		_ = os.RemoveAll(filepath.Join(f.rootPath))
	}
}

func prepareQuery(query string) string {
	query = text.Truncate(query, 100, "")
	query = strings.ToLower(query)
	query = strings.TrimSpace(query)
	var escapedQuery strings.Builder

	for _, char := range query {
		if _, found := specialChars[char]; !found {
			escapedQuery.WriteRune(char)
		}
	}

	resultQuery := escapedQuery.String()
	if resultQuery == "" {
		return resultQuery
	}
	return "(\"" + resultQuery + "\" OR " + resultQuery + ")"
}
