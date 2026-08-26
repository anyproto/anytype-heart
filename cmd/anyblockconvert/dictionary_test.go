package main

// dictionary_test.go pins the §2f import wiring end to end: a bundle whose
// only declaration of a property is the dictionary still converts — the
// entry feeds the format table (so the undeclared-format gate passes and the
// value decodes) and the property is minted up front, with the FULL declared
// shape, whether or not any type lists it.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// How this can fail, piece by piece of the main.go wiring: skip reading
// properties.json (CheckPropertyFormats reports the key undeclared and the
// run errors); read it but do not merge into the format table (same); merge
// but drop the pre-mint loop (the value decodes but NO relation object
// exists in the archive — the outDir assertion goes red); or shed one of
// the five §2e members between the entry and mintRelation (the details
// assertions catch the seam).
func TestRun_DictionaryDeclaredPropertyConverts(t *testing.T) {
	// given: a bundle with no type documents at all — the dictionary is the
	// only declaration the property has
	inDir := t.TempDir()
	outDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(inDir, "properties.json"), []byte(`{"version":1,
		"properties":[
			{"property":"6a32d4856761631534b22f85","name":"Budget","format":"number",
			 "description":"Planned spend","max_count":1,"readonly":true,"default_value":100}]}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(inDir, "objects"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(inDir, "objects", "a.json"), []byte(`{"version":1,
		"id":"obj-a",
		"property_internal_keys":{"budget":"6a32d4856761631534b22f85"},
		"properties":{"name":"A","budget":250}}`), 0o644))

	// when
	require.NoError(t, run(inDir, outDir, false, false, formatPb))

	// then: the relation object exists in the archive, with the whole shape
	relPath := ""
	filepath.Walk(filepath.Join(outDir, "relations"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			relPath = p
		}
		return nil
	})
	require.NotEmpty(t, relPath, "the dictionary-declared property must be minted without any type listing it")
	data, err := os.ReadFile(relPath)
	require.NoError(t, err)
	var sw pb.SnapshotWithType
	require.NoError(t, proto.Unmarshal(data, &sw))
	det := sw.Snapshot.GetData().GetDetails().GetFields()
	assert.Equal(t, "Budget", det["name"].GetStringValue())
	assert.Equal(t, float64(model.RelationFormat_number), det["relationFormat"].GetNumberValue())
	assert.Equal(t, "Planned spend", det["description"].GetStringValue())
	assert.Equal(t, float64(1), det["relationMaxCount"].GetNumberValue())
	assert.True(t, det["relationReadonlyValue"].GetBoolValue())
	assert.Equal(t, float64(100), det["relationDefaultValue"].GetNumberValue())

	// and the value decoded against the declared format
	objData, err := os.ReadFile(filepath.Join(outDir, "objects", "obj-a.pb"))
	require.NoError(t, err)
	var obj pb.SnapshotWithType
	require.NoError(t, proto.Unmarshal(objData, &obj))
	assert.Equal(t, float64(250),
		obj.Snapshot.GetData().GetDetails().GetFields()["6a32d4856761631534b22f85"].GetNumberValue())
}
