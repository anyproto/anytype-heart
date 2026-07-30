package anyblockbatch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeDocs lays out a bundle in a temp dir and returns its files, sorted the
// way DiscoverJSONFiles returns them.
func writeDocs(t *testing.T, docs map[string]string) []string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range docs {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	files, err := DiscoverJSONFiles(dir)
	require.NoError(t, err)
	return files
}

const wikiPageType = `{"version": 1, "kind": "objectType", "key": "wikiPage", "id": "type-wiki-page"}`

func TestCheckTemplateTargets_ResolvableTargetPasses(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": wikiPageType,
		"templates/article.json":    `{"version": 1, "type": "template", "templateFor": "wikiPage"}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)

	bad, err := CheckTemplateTargets(files, typeIds)
	require.NoError(t, err)
	assert.Empty(t, bad)
}

// a bundled type key is not enough: UpdateObjectIDsInRelations passes bundled
// ids through untouched, so "_otpage" would survive import literally and match
// no type in the space
func TestCheckTemplateTargets_BundledTargetIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"templates/note.json": `{"version": 1, "type": "template", "templateFor": "page"}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "page", bad[0].Target)
	assert.Contains(t, ReportTemplateTargets(bad), "bundled")
}

func TestCheckTemplateTargets_UndefinedTargetIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"templates/article.json": `{"version": 1, "type": "template", "templateFor": "wikiPage"}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Equal(t, "wikiPage", bad[0].Target)
}

// a type document with no id has nothing for the detail to point at
func TestCheckTemplateTargets_IdlessTargetIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": `{"version": 1, "kind": "objectType", "key": "wikiPage"}`,
		"templates/article.json":    `{"version": 1, "type": "template", "templateFor": "wikiPage"}`,
	})
	typeIds, err := TypeIds(files)
	require.NoError(t, err)

	bad, err := CheckTemplateTargets(files, typeIds)
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Contains(t, bad[0].Reason, "no \"id\"")
}

// a template naming no target type at all belongs to nothing — the app refuses
// to create one (objectcreator.createTemplate), and import does not
func TestCheckTemplateTargets_MissingTemplateForIsReported(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"templates/orphan.json": `{"version": 1, "type": "template"}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	require.Len(t, bad, 1)
	assert.Empty(t, bad[0].Target)
}

// an authored targetObjectType is the value the converter keeps, so the
// document is wired whatever templateFor resolves to
func TestCheckTemplateTargets_AuthoredTargetObjectTypePasses(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"templates/article.json": `{"version": 1, "type": "template", "templateFor": "page",
		  "properties": {"targetObjectType": "type-page"}}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	assert.Empty(t, bad)
}

func TestCheckTemplateTargets_NonTemplatesAreIgnored(t *testing.T) {
	files := writeDocs(t, map[string]string{
		"types/wiki-page.type.json": wikiPageType,
		"objects/page.json":         `{"version": 1, "type": "wikiPage"}`,
	})
	bad, err := CheckTemplateTargets(files, map[string]string{})
	require.NoError(t, err)
	assert.Empty(t, bad)
}
