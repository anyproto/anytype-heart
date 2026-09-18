package ftsearch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	tantivy "github.com/anyproto/tantivy-go"
	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/wallet"
)

// schemaField mirrors one entry of the "schema" array in tantivy's meta.json
type schemaField struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Options struct {
		Indexing struct {
			Record     string `json:"record"`
			Fieldnorms bool   `json:"fieldnorms"`
			Tokenizer  string `json:"tokenizer"`
		} `json:"indexing"`
		Stored bool `json:"stored"`
		Fast   bool `json:"fast"`
	} `json:"options"`
}

func textField(name, record, tokenizer string, fast bool) schemaField {
	f := schemaField{Name: name, Type: "text"}
	f.Options.Indexing.Record = record
	f.Options.Indexing.Fieldnorms = true
	f.Options.Indexing.Tokenizer = tokenizer
	f.Options.Stored = true
	f.Options.Fast = fast
	return f
}

// pinnedSchema is the schema Run builds under the current ftsVer. If
// TestSchemaPinned fails, the schema changed: every existing index would be
// refused by tantivy and rebuilt through the quarantine path. Decide
// deliberately whether that is intended (then update this list) or whether
// the change belongs behind a ftsVer bump. Only what tantivy serializes is
// pinned: the builder's "indexed" flag is not part of meta.json.
var pinnedSchema = []schemaField{
	textField(fieldId, "position", tokenizerId, false),
	textField(fieldIdRaw, "basic", "raw", true),
	textField(fieldSpace, "basic", "raw", true),
	textField(fieldTitle, "position", "simple_tokenizer", false),
	textField(fieldTitleZh, "position", "jieba", false),
	textField(fieldText, "position", "simple_tokenizer", false),
	textField(fieldTextZh, "position", "jieba", false),
	textField(fieldAuthor, "basic", "raw", true),
	textField(fieldOrderId, "basic", "raw", true),
	textField(fieldMessageId, "basic", "raw", true),
	textField(fieldTimestamp, "basic", "raw", true),
}

type legacyField struct {
	name      string
	stored    bool
	indexed   bool
	fast      bool
	record    int
	tokenizer string
}

// writeIndexWithFields creates a tantivy index at the current ftsVer path
// with the given schema, simulating an account written by another build.
// It uses the current tantivy-go, not the one that release shipped: what
// matters for the open check is the recorded schema, not the byte history.
func writeIndexWithFields(t *testing.T, repoPath string, fields []legacyField) {
	t.Helper()
	require.NoError(t, tantivy.LibInit(false, true, "release"))
	builder, err := tantivy.NewSchemaBuilder()
	require.NoError(t, err)
	for _, f := range fields {
		require.NoError(t, builder.AddTextField(f.name, f.stored, f.indexed, f.fast, f.record, f.tokenizer))
	}
	schema, err := builder.BuildSchema()
	require.NoError(t, err)
	idx, err := tantivy.NewTantivyContextWithSchema(filepath.Join(repoPath, ftsDir2, ftsVer), schema)
	require.NoError(t, err)
	defer idx.Free()
	// seed one document so a rebuild is observable (data gone, opstamp reset)
	require.NoError(t, idx.RegisterTextAnalyzerSimple(tantivy.TokenizerSimple, 40, tantivy.English))
	require.NoError(t, idx.RegisterTextAnalyzerJieba(tantivy.TokenizerJieba, 40))
	require.NoError(t, idx.RegisterTextAnalyzerSimple(tokenizerId, 1000, tantivy.English))
	require.NoError(t, idx.RegisterTextAnalyzerRaw(tantivy.TokenizerRaw))
	doc := tantivy.NewDocument()
	require.NoError(t, doc.AddFields(legacyDocId, idx, fieldId, fieldIdRaw))
	require.NoError(t, doc.AddField(legacySpaceId, idx, fieldSpace))
	require.NoError(t, doc.AddFields(legacyTitle, idx, fieldTitle, fieldTitleZh))
	require.NoError(t, doc.AddFields("legacy text", idx, fieldText, fieldTextZh))
	_, err = idx.AddAndConsumeDocumentsWithOpstamp(doc)
	require.NoError(t, err)
}

const (
	legacyDocId   = "legacy/b/1"
	legacySpaceId = "s"
	legacyTitle   = "legacydoc"
)

// sevenFieldSchema is the schema shipped in v0.46.0/v0.47.2 (no message
// fields), copied from that release's Run
var sevenFieldSchema = []legacyField{
	{fieldId, true, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tokenizerId},
	{fieldIdRaw, true, true, true, tantivy.IndexRecordOptionBasic, tantivy.TokenizerRaw},
	{fieldSpace, true, false, true, tantivy.IndexRecordOptionBasic, tantivy.TokenizerRaw},
	{fieldTitle, true, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerSimple},
	{fieldTitleZh, true, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerJieba},
	{fieldText, true, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerSimple},
	{fieldTextZh, true, true, false, tantivy.IndexRecordOptionWithFreqsAndPositions, tantivy.TokenizerJieba},
}

// sameNamesDifferentOptionsSchema has the current field names but Title is
// tokenized raw: invisible to the meta.json name check, refused by tantivy
func sameNamesDifferentOptionsSchema() []legacyField {
	fields := append([]legacyField(nil), sevenFieldSchema...)
	fields[3].tokenizer = tantivy.TokenizerRaw
	fields[3].record = tantivy.IndexRecordOptionBasic
	for _, name := range []string{fieldAuthor, fieldOrderId, fieldMessageId, fieldTimestamp} {
		fields = append(fields, legacyField{name, true, false, true, tantivy.IndexRecordOptionBasic, tantivy.TokenizerRaw})
	}
	return fields
}

func startFts(t *testing.T, repoPath string) (FTSearch, *app.App, error) {
	t.Helper()
	ft := TantivyNew()
	ta := new(app.App)
	ta.Register(wallet.NewWithRepoDirAndRandomKeys(repoPath)).Register(ft)
	err := ta.Start(context.Background())
	return ft, ta, err
}

func hasQuarantineDir(repoPath string) bool {
	matches, err := filepath.Glob(filepath.Join(repoPath, ftsDir2+quarantineSuffix+"*"))
	if err != nil {
		panic(err) // a bad pattern must not read as "no quarantine dir"
	}
	return len(matches) > 0
}

// requireRebuiltIndex asserts the legacy index seeded by writeIndexWithFields
// was discarded and replaced by an empty, working one
func requireRebuiltIndex(t *testing.T, ft FTSearch, wantRecovery string) {
	t.Helper()
	assert.Equal(t, wantRecovery, ft.(*ftSearch).schemaRecovery)
	seq, err := ft.LastDbState()
	require.NoError(t, err)
	assert.Zero(t, seq, "a rebuilt index must report opstamp 0 so ftInit re-enqueues everything")
	res, err := ft.Search(legacySpaceId, legacyTitle, 0, false)
	require.NoError(t, err)
	assert.Empty(t, res, "legacy documents must be gone")
	require.NoError(t, ft.Index(SearchDoc{Id: "obj/b/1", SpaceId: "s", Title: "hello", Text: "world"}))
	res, err = ft.Search("s", "hello", 0, false)
	require.NoError(t, err)
	require.Len(t, res, 1)
}

func requireQuarantineSwept(t *testing.T, repoPath string) {
	t.Helper()
	assert.Eventually(t, func() bool { return !hasQuarantineDir(repoPath) },
		5*time.Second, 50*time.Millisecond, "quarantined index must be removed")
}

func TestRun_RebuildsIndexWithFewerFields(t *testing.T) {
	// given: an account last written by v0.47.2
	repoPath := t.TempDir()
	writeIndexWithFields(t, repoPath, sevenFieldSchema)

	// when
	ft, ta, err := startFts(t, repoPath)

	// then: tantivy refuses it, it is quarantined, rebuilt, quarantine removed
	require.NoError(t, err)
	defer func() { _ = ta.Close(context.Background()) }()
	requireRebuiltIndex(t, ft, schemaRecoveryTantivyError)
	assert.True(t, ft.ConsistencyReport().Rebuilt)
	requireQuarantineSwept(t, repoPath)
}

func TestRun_RebuildsIndexWithFewerFieldsWhenErrorTextIsUnknown(t *testing.T) {
	// given: tantivy's error text no longer matches (library upgrade)
	marker := schemaMismatchMarker
	schemaMismatchMarker = "no such text"
	t.Cleanup(func() { schemaMismatchMarker = marker })
	repoPath := t.TempDir()
	writeIndexWithFields(t, repoPath, sevenFieldSchema)

	// when
	ft, ta, err := startFts(t, repoPath)

	// then: the field names recorded in meta.json still identify the mismatch
	require.NoError(t, err)
	defer func() { _ = ta.Close(context.Background()) }()
	requireRebuiltIndex(t, ft, schemaRecoveryMetaNames)
	requireQuarantineSwept(t, repoPath)
}

func TestRun_RebuildsIndexWithDifferentFieldOptions(t *testing.T) {
	// given: same field names, different tokenizer (only tantivy can tell)
	repoPath := t.TempDir()
	writeIndexWithFields(t, repoPath, sameNamesDifferentOptionsSchema())

	// when
	ft, ta, err := startFts(t, repoPath)

	// then: recovered from tantivy's schema error
	require.NoError(t, err)
	defer func() { _ = ta.Close(context.Background()) }()
	requireRebuiltIndex(t, ft, schemaRecoveryTantivyError)
	requireQuarantineSwept(t, repoPath)
}

func TestRun_NonSchemaOpenErrorIsNotQuarantined(t *testing.T) {
	// given: the index path cannot be opened for a reason unrelated to the schema
	repoPath := t.TempDir()
	indexPath := filepath.Join(repoPath, ftsDir2, ftsVer)
	require.NoError(t, os.MkdirAll(filepath.Dir(indexPath), 0o700))
	require.NoError(t, os.WriteFile(indexPath, []byte("not a directory"), 0o600))

	// when
	_, _, err := startFts(t, repoPath)

	// then: the error propagates, nothing is renamed or deleted
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrIndexInUse)
	assert.False(t, hasQuarantineDir(repoPath))
	require.FileExists(t, indexPath)
}

func TestRun_RebuildsIndexWithUndecodableMeta(t *testing.T) {
	// given: meta.json damaged (e.g. crash mid-write), no other process
	repoPath := t.TempDir()
	writeIndexWithFields(t, repoPath, sevenFieldSchema)
	metaPath := filepath.Join(repoPath, ftsDir2, ftsVer, "meta.json")
	require.NoError(t, os.WriteFile(metaPath, []byte("{garbage"), 0o600))

	// when
	ft, ta, err := startFts(t, repoPath)

	// then: rebuilt from the local decode verdict, not from tantivy's text
	require.NoError(t, err)
	defer func() { _ = ta.Close(context.Background()) }()
	requireRebuiltIndex(t, ft, schemaRecoveryMetaUndecodable)
	requireQuarantineSwept(t, repoPath)
}

func TestRun_MissingMetaIsNotQuarantined(t *testing.T) {
	// given: segments present but meta.json absent (read error, not decode error)
	repoPath := t.TempDir()
	writeIndexWithFields(t, repoPath, sevenFieldSchema)
	require.NoError(t, os.Remove(filepath.Join(repoPath, ftsDir2, ftsVer, "meta.json")))

	// when
	_, _, err := startFts(t, repoPath)

	// then: a fresh index is created in place, nothing renamed
	require.NoError(t, err)
	assert.False(t, hasQuarantineDir(repoPath))
}

func TestRun_KeepsIndexWithCurrentSchema(t *testing.T) {
	// given: an index written by this build with a document in it
	repoPath := t.TempDir()
	ft, ta, err := startFts(t, repoPath)
	require.NoError(t, err)
	require.NoError(t, ft.Index(SearchDoc{Id: "obj/b/1", SpaceId: "s", Title: "keep me"}))
	require.NoError(t, ta.Close(context.Background()))

	// when: reopened
	ft, ta, err = startFts(t, repoPath)

	// then: nothing is rebuilt, the document survives
	require.NoError(t, err)
	defer func() { _ = ta.Close(context.Background()) }()
	assert.False(t, hasQuarantineDir(repoPath))
	assert.Empty(t, ft.(*ftSearch).schemaRecovery)
	assert.False(t, ft.ConsistencyReport().Rebuilt)
	res, err := ft.Search("s", "keep", 0, false)
	require.NoError(t, err)
	require.Len(t, res, 1)
	seq, err := ft.LastDbState()
	require.NoError(t, err)
	assert.NotZero(t, seq)
}

func lockWriter(t *testing.T, repoPath string) {
	t.Helper()
	lock := flock.New(filepath.Join(repoPath, ftsDir2, ftsVer, ".tantivy-writer.lock"))
	locked, err := lock.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	t.Cleanup(func() { _ = lock.Unlock() })
}

func TestRun_LockedIndexIsNotRebuilt(t *testing.T) {
	// given: another process holds the writer lock on an incompatible index
	repoPath := t.TempDir()
	writeIndexWithFields(t, repoPath, sevenFieldSchema)
	lockWriter(t, repoPath)

	// when
	_, _, err := startFts(t, repoPath)

	// then: the lock wins over schema recovery, nothing is quarantined
	require.ErrorIs(t, err, ErrIndexInUse)
	assert.False(t, hasQuarantineDir(repoPath))
	_, err = os.Stat(filepath.Join(repoPath, ftsDir2, ftsVer, "meta.json"))
	require.NoError(t, err)
}

func TestRun_LockedIndexWithCorruptMetaIsNotTouched(t *testing.T) {
	// given: a held writer lock and an unreadable meta.json
	repoPath := t.TempDir()
	writeIndexWithFields(t, repoPath, sevenFieldSchema)
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, ftsDir2, ftsVer, "meta.json"), []byte("{garbage"), 0o600))
	lockWriter(t, repoPath)

	// when
	_, _, err := startFts(t, repoPath)

	// then
	require.ErrorIs(t, err, ErrIndexInUse)
	assert.False(t, hasQuarantineDir(repoPath))
}

func TestRun_SweepsOrphanedQuarantineDirs(t *testing.T) {
	// given: a quarantine copy left behind by an interrupted earlier start
	repoPath := t.TempDir()
	orphan := filepath.Join(repoPath, ftsDir2+quarantineSuffix+"123")
	require.NoError(t, os.MkdirAll(orphan, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(orphan, "meta.json"), []byte("{}"), 0o600))

	// when
	_, ta, err := startFts(t, repoPath)

	// then
	require.NoError(t, err)
	defer func() { _ = ta.Close(context.Background()) }()
	assert.Eventually(t, func() bool { return !hasQuarantineDir(repoPath) },
		5*time.Second, 50*time.Millisecond, "orphaned quarantine dir must be swept")
}

func TestQuarantineRefusesUnexpectedPath(t *testing.T) {
	// given: an existing directory that is not the fts root (rename would succeed)
	root := filepath.Join(t.TempDir(), "something-else")
	require.NoError(t, os.MkdirAll(root, 0o700))
	ft := &ftSearch{rootPath: root}

	// when
	_, err := ft.quarantineCorruptIndex()

	// then: refused, nothing moved
	require.ErrorContains(t, err, "refusing to quarantine")
	require.DirExists(t, root)
}

func TestIsSchemaMismatchError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"tantivy schema mismatch", errors.New("Schema error: 'An index exists but the schema does not match.'"), true},
		{"io error", errors.New("An IO error occurred: permission denied"), false},
		{"lock error", errors.New("Failed to acquire Lockfile"), false},
		{"corrupt meta", errors.New("Data corrupted: meta.json"), false},
		{"other schema error", errors.New("Schema error: 'field not found'"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, isSchemaMismatchError(c.err))
		})
	}
}

func TestSchemaPinned(t *testing.T) {
	repoPath := t.TempDir()
	_, ta, err := startFts(t, repoPath)
	require.NoError(t, err)
	defer func() { _ = ta.Close(context.Background()) }()

	raw, err := os.ReadFile(filepath.Join(repoPath, ftsDir2, ftsVer, "meta.json"))
	require.NoError(t, err)
	var meta struct {
		Schema []schemaField `json:"schema"`
	}
	require.NoError(t, json.Unmarshal(raw, &meta))
	assert.Equal(t, pinnedSchema, meta.Schema,
		"tantivy schema changed under ftsVer %q: existing indexes will be rebuilt on next start; update pinnedSchema if intended", ftsVer)

	names := make([]string, 0, len(meta.Schema))
	for _, f := range meta.Schema {
		names = append(names, f.Name)
	}
	assert.Equal(t, schemaFieldNames, names, "schemaFieldNames must match the built schema")
}
