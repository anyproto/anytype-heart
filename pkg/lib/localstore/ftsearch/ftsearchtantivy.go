package ftsearch

/*
#cgo windows,amd64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/windows-amd64 -ltantivy_go -lm -pthread -lws2_32 -lbcrypt -lwsock32 -lntdll -luserenv -lsynchronization
#cgo darwin,amd64 LDFLAGS:-L${SRCDIR}/../../../../deps/libs/darwin-amd64 -ltantivy_go -lm -pthread -framework CoreFoundation -framework Security -ldl
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
	"github.com/anyproto/tantivy-go/go/tantivy"
	_ "github.com/anyproto/tantivy-go/go/tantivy"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
)

func TantivyNew() FTSearch {
	return new(ftSearch2)
}

type ftSearch2 struct {
	rootPath  string
	ftsPath   string
	builderId string
	index     *tantivy.Index
	schema    *tantivy.Schema
}

func (f *ftSearch2) BatchDeleteObjects(ids []string) error {
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

func (f *ftSearch2) DeleteObject(objectId string) error {
	return f.index.DeleteDocuments(fieldIdRaw, objectId)
}

var ftsDir2 = "fts_tantivy"

func (f *ftSearch2) Init(a *app.App) error {
	repoPath := a.MustComponent(wallet.CName).(wallet.Wallet).RepoPath()
	f.rootPath = filepath.Join(repoPath, ftsDir2)
	f.ftsPath = filepath.Join(repoPath, ftsDir2, ftsVer)
	tantivy.LibInit("debug")
	return nil
}

func (f *ftSearch2) Name() (name string) {
	return CName
}

func (f *ftSearch2) Run(context.Context) error {
	builder, err := tantivy.NewSchemaBuilder()
	if err != nil {
		fmt.Println("Failed to create schema builder:", err)
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
		tantivy.TokenizerEdgeNgram,
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
		fmt.Println("Failed to build schema:", err)
		return err
	}

	index, err := tantivy.NewIndexWithSchema(f.ftsPath, schema)
	if err != nil {
		fmt.Println("Failed to create index:", err)
		return err
	}
	f.schema = schema
	f.index = index

	f.cleanupBleve()

	err = index.RegisterTextAnalyzerSimple(tantivy.TokenizerSimple, 40, tantivy.English)
	if err != nil {
		fmt.Println("Failed to register text analyzer:", err)
		return err
	}

	err = index.RegisterTextAnalyzerSimple(tokenizerId, 1000, tantivy.English)
	if err != nil {
		fmt.Println("Failed to register text analyzer:", err)
		return err
	}

	err = index.RegisterTextAnalyzerEdgeNgram(tantivy.TokenizerEdgeNgram, 1, 5, 100)
	if err != nil {
		fmt.Println("Failed to register text analyzer:", err)
		return err
	}

	err = index.RegisterTextAnalyzerRaw(tantivy.TokenizerRaw)
	if err != nil {
		fmt.Println("Failed to register text analyzer:", err)
		return err
	}

	return nil
}

func (f *ftSearch2) Index(doc SearchDoc) error {
	metrics.ObjectFTUpdatedCounter.Inc()
	tantivyDoc, err := f.convertDoc(doc)
	if err != nil {
		return err
	}

	res := f.index.AddAndConsumeDocuments(tantivyDoc)
	return res
}

func (f *ftSearch2) convertDoc(doc SearchDoc) (*tantivy.Document, error) {
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

func (f *ftSearch2) BatchIndex(ctx context.Context, docs []SearchDoc, deletedDocs []string) (err error) {
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

func (f *ftSearch2) Search(spaceId string, highlightFormatter HighlightFormatter, query string) (results search.DocumentMatchCollection, err error) {
	start := time.Now().UnixMilli()
	if spaceId != "" {
		query = fmt.Sprintf("%s:%s AND %s", fieldSpace, spaceId, escapeQuery(query))
	} else {
		query = escapeQuery(query)
	}
	result, err := f.index.Search(query, 100, true, fieldId, fieldSpace, fieldTitle, fieldText)
	fmt.Println("### search took", time.Now().UnixMilli()-start, "ms")
	if err != nil {
		return nil, err
	}
	var p fastjson.Parser
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
					fragments[fieldTitle] = append(fragments[fieldTitle], string(object.Get("fragment").MarshalTo(nil)))
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

func (f *ftSearch2) Delete(id string) error {
	return f.BatchDeleteObjects([]string{id})
}

func (f *ftSearch2) DocCount() (uint64, error) {
	return f.index.NumDocs()
}

func (f *ftSearch2) Close(ctx context.Context) error {
	f.schema = nil
	f.index.Free()
	return nil
}

func (f *ftSearch2) cleanupBleve() {
	_ = os.RemoveAll(filepath.Join(f.rootPath, ftsDir))
}

func escapeQuery(query string) string {
	specialChars := []rune{'+', '-', '&', '|', '!', '(', ')', '{', '}', '[', ']', '^', '"', '~', '*', '?', ':'}

	var escapedQuery strings.Builder

	for _, char := range query {
		if contains(specialChars, char) {
			escapedQuery.WriteRune('\\')
		}
		escapedQuery.WriteRune(char)
	}

	return escapedQuery.String()
}

func contains(slice []rune, char rune) bool {
	for _, item := range slice {
		if item == char {
			return true
		}
	}
	return false
}
