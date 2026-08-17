package bundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func writeExtraRelationsFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadExtraRelations(t *testing.T) {
	t.Run("adds relation with all fields", func(t *testing.T) {
		// given
		path := writeExtraRelationsFile(t, t.TempDir(), "extra.json", `[
			{
				"key": "internalTraceId",
				"name": "Internal trace ID",
				"format": "shorttext",
				"source": "details",
				"description": "Correlates an object with an internal trace",
				"maxCount": 1,
				"hidden": true,
				"readonly": true,
				"revision": 2,
				"includeTime": true
			}
		]`)
		dst := map[domain.RelationKey]*model.Relation{}
		want := map[domain.RelationKey]*model.Relation{
			"internalTraceId": {
				Id:               "_brinternalTraceId",
				Key:              "internalTraceId",
				Name:             "Internal trace ID",
				Format:           model.RelationFormat_shorttext,
				DataSource:       model.Relation_details,
				Description:      "Correlates an object with an internal trace",
				MaxCount:         1,
				Hidden:           true,
				ReadOnly:         true,
				ReadOnlyRelation: true,
				Scope:            model.Relation_type,
				Revision:         2,
				IncludeTime:      true,
			},
		}

		// when
		err := loadExtraRelations([]string{path}, dst)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, dst)
	})

	t.Run("prefixes object types with type prefix", func(t *testing.T) {
		// given
		path := writeExtraRelationsFile(t, t.TempDir(), "extra.json", `[
			{"key": "owner", "name": "Owner", "format": "object", "source": "details", "objectTypes": ["profile", "participant"]}
		]`)
		dst := map[domain.RelationKey]*model.Relation{}

		// when
		err := loadExtraRelations([]string{path}, dst)

		// then
		require.NoError(t, err)
		require.NotNil(t, dst["owner"])
		assert.Equal(t, []string{"_otprofile", "_otparticipant"}, dst["owner"].ObjectTypes)
	})

	t.Run("omitted optional fields stay zero", func(t *testing.T) {
		// given
		path := writeExtraRelationsFile(t, t.TempDir(), "extra.json", `[
			{"key": "minimal", "name": "Minimal", "format": "longtext", "source": "local"}
		]`)
		dst := map[domain.RelationKey]*model.Relation{}
		want := &model.Relation{
			Id:               "_brminimal",
			Key:              "minimal",
			Name:             "Minimal",
			Format:           model.RelationFormat_longtext,
			DataSource:       model.Relation_local,
			ReadOnlyRelation: true,
			Scope:            model.Relation_type,
		}

		// when
		err := loadExtraRelations([]string{path}, dst)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, dst["minimal"])
	})

	t.Run("accepts key with leading underscore", func(t *testing.T) {
		// given
		path := writeExtraRelationsFile(t, t.TempDir(), "extra.json", `[
			{"key": "_custom_score", "name": "Custom score", "format": "number", "source": "derived"}
		]`)
		dst := map[domain.RelationKey]*model.Relation{}

		// when
		err := loadExtraRelations([]string{path}, dst)

		// then
		require.NoError(t, err)
		require.NotNil(t, dst["_custom_score"])
		assert.Equal(t, "_br_custom_score", dst["_custom_score"].Id)
	})

	t.Run("existing relation is never overridden", func(t *testing.T) {
		// given
		path := writeExtraRelationsFile(t, t.TempDir(), "extra.json", `[
			{"key": "name", "name": "Hijacked", "format": "number", "source": "local"}
		]`)
		existing := &model.Relation{Id: "_brname", Key: "name", Name: "Name", Format: model.RelationFormat_shorttext}
		dst := map[domain.RelationKey]*model.Relation{"name": existing}

		// when
		err := loadExtraRelations([]string{path}, dst)

		// then
		require.NoError(t, err)
		require.Len(t, dst, 1)
		assert.Same(t, existing, dst["name"])
	})

	t.Run("merges relations from several files", func(t *testing.T) {
		// given
		dir := t.TempDir()
		first := writeExtraRelationsFile(t, dir, "a.json", `[
			{"key": "alpha", "name": "Alpha", "format": "longtext", "source": "details"}
		]`)
		second := writeExtraRelationsFile(t, dir, "b.json", `[
			{"key": "beta", "name": "Beta", "format": "number", "source": "details"}
		]`)
		dst := map[domain.RelationKey]*model.Relation{}

		// when
		err := loadExtraRelations([]string{first, second}, dst)

		// then
		require.NoError(t, err)
		assert.Len(t, dst, 2)
		assert.Contains(t, dst, domain.RelationKey("alpha"))
		assert.Contains(t, dst, domain.RelationKey("beta"))
	})

	t.Run("empty path list is a no-op", func(t *testing.T) {
		// given
		dst := map[domain.RelationKey]*model.Relation{}

		// when
		err := loadExtraRelations(nil, dst)

		// then
		require.NoError(t, err)
		assert.Empty(t, dst)
	})

	t.Run("same key in two files is an error naming both", func(t *testing.T) {
		// given
		dir := t.TempDir()
		first := writeExtraRelationsFile(t, dir, "a.json", `[
			{"key": "dup", "name": "Dup", "format": "longtext", "source": "details"}
		]`)
		second := writeExtraRelationsFile(t, dir, "b.json", `[
			{"key": "dup", "name": "Dup again", "format": "number", "source": "details"}
		]`)
		dst := map[domain.RelationKey]*model.Relation{}

		// when
		err := loadExtraRelations([]string{first, second}, dst)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), first)
		assert.Contains(t, err.Error(), second)
		assert.Contains(t, err.Error(), "dup")
	})

	t.Run("missing file is an error naming the path", func(t *testing.T) {
		// given
		path := filepath.Join(t.TempDir(), "absent.json")
		dst := map[domain.RelationKey]*model.Relation{}

		// when
		err := loadExtraRelations([]string{path}, dst)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), path)
	})

	t.Run("malformed json is an error naming the path", func(t *testing.T) {
		// given
		path := writeExtraRelationsFile(t, t.TempDir(), "extra.json", `{"key": "notAnArray"}`)
		dst := map[domain.RelationKey]*model.Relation{}

		// when
		err := loadExtraRelations([]string{path}, dst)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), path)
	})

	t.Run("invalid entries are errors", func(t *testing.T) {
		for name, entry := range map[string]string{
			"unknown format":      `{"key": "x", "name": "X", "format": "hypertext", "source": "details"}`,
			"unknown source":      `{"key": "x", "name": "X", "format": "longtext", "source": "cloud"}`,
			"missing format":      `{"key": "x", "name": "X", "source": "details"}`,
			"missing source":      `{"key": "x", "name": "X", "format": "longtext"}`,
			"empty key":           `{"key": "", "name": "X", "format": "longtext", "source": "details"}`,
			"key with dash":       `{"key": "my-key", "name": "X", "format": "longtext", "source": "details"}`,
			"key with space":      `{"key": "my key", "name": "X", "format": "longtext", "source": "details"}`,
			"key starting digit":  `{"key": "1key", "name": "X", "format": "longtext", "source": "details"}`,
			"empty name":          `{"key": "x", "name": "", "format": "longtext", "source": "details"}`,
			"negative maxCount":   `{"key": "x", "name": "X", "format": "longtext", "source": "details", "maxCount": -1}`,
			"huge maxCount":       `{"key": "x", "name": "X", "format": "longtext", "source": "details", "maxCount": 3000000000}`,
			"empty object type":   `{"key": "x", "name": "X", "format": "object", "source": "details", "objectTypes": [""]}`,
			"prefixed objectType": `{"key": "x", "name": "X", "format": "object", "source": "details", "objectTypes": ["_otpage"]}`,
		} {
			t.Run(name, func(t *testing.T) {
				// given
				path := writeExtraRelationsFile(t, t.TempDir(), "extra.json", "["+entry+"]")
				dst := map[domain.RelationKey]*model.Relation{}

				// when
				err := loadExtraRelations([]string{path}, dst)

				// then
				require.Error(t, err)
				assert.Empty(t, dst)
			})
		}
	})
}

// envInitChildMarker tells a subprocess that it is the child of TestExtraRelationsInit, so package
// init has already run with ANYTYPE_EXTRA_RELATIONS set.
const envInitChildMarker = "ANYTYPE_EXTRA_RELATIONS_INIT_CHILD"

const extraInitTestRelation = domain.RelationKey("extraInitTestRelation")

func TestExtraRelationsInitChild(t *testing.T) {
	if os.Getenv(envInitChildMarker) == "" {
		t.Skip("runs only as a subprocess of TestExtraRelationsInit")
	}

	// then
	relation, err := GetRelation(extraInitTestRelation)
	require.NoError(t, err)
	assert.Equal(t, "_brextraInitTestRelation", relation.Id)
	assert.True(t, HasRelation(extraInitTestRelation))
	// the merge must happen before init derives the key lists, otherwise a local relation would
	// be missing from LocalRelationsKeys
	assert.Contains(t, LocalRelationsKeys, extraInitTestRelation)
}

func runInitChild(t *testing.T, extraPath string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run", "^TestExtraRelationsInitChild$", "-test.v")
	cmd.Env = append(os.Environ(), envInitChildMarker+"=1", envExtraRelations+"="+extraPath)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestExtraRelationsInit(t *testing.T) {
	t.Run("relations from the env file join the bundle before key lists are derived", func(t *testing.T) {
		// given
		path := writeExtraRelationsFile(t, t.TempDir(), "extra.json", `[
			{"key": "extraInitTestRelation", "name": "Extra init test", "format": "longtext", "source": "local"}
		]`)

		// when
		out, err := runInitChild(t, path)

		// then
		require.NoError(t, err, out)
	})

	t.Run("invalid file aborts startup naming the env var", func(t *testing.T) {
		// given
		path := writeExtraRelationsFile(t, t.TempDir(), "extra.json", `[
			{"key": "extraInitTestRelation", "name": "Extra init test", "format": "hypertext", "source": "local"}
		]`)

		// when
		out, err := runInitChild(t, path)

		// then
		require.Error(t, err)
		assert.Contains(t, out, envExtraRelations)
		assert.Contains(t, out, "hypertext")
	})
}

func TestExtraRelationsPathsFromEnv(t *testing.T) {
	t.Run("unset env yields no paths", func(t *testing.T) {
		// given
		t.Setenv(envExtraRelations, "")

		// when
		got := extraRelationsPaths()

		// then
		assert.Empty(t, got)
	})

	t.Run("splits env by the os path list separator", func(t *testing.T) {
		// given
		joined := strings.Join([]string{"/tmp/a.json", "/tmp/b.json"}, string(os.PathListSeparator))
		t.Setenv(envExtraRelations, joined)
		want := []string{"/tmp/a.json", "/tmp/b.json"}

		// when
		got := extraRelationsPaths()

		// then
		assert.Equal(t, want, got)
	})
}
