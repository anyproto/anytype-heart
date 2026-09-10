package export

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestExportName(t *testing.T) {
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, objectstore.TestTechSpaceId, []spaceindex.TestObject{{
		bundle.RelationKeyId:             domain.String("space-view"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
		bundle.RelationKeyName:           domain.String("My Space"),
	}})
	docs := Docs{
		"object": {Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String("Project Plan"),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		})},
		"dependency": {Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("Linked object"),
		})},
		"untitled": {Details: domain.NewDetails()},
	}
	date := time.Date(2026, 9, 9, 16, 35, 0, 123000000, time.UTC)
	for _, tc := range []struct {
		name, space string
		ids         []string
		want        string
	}{
		{"whole space", spaceId, nil, "my-space"},
		{"single root with dependencies", spaceId, []string{"object"}, "my-space-project-plan"},
		{"multiple roots", spaceId, []string{"object", "dependency"}, "my-space"},
		{"untitled object", spaceId, []string{"untitled"}, "my-space-untitled"},
		{"all spaces", "", nil, "all-spaces"},
		{"infer object's space", "", []string{"object"}, "my-space-project-plan"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &exportContext{
				export:  &export{objectStore: store},
				spaceId: tc.space,
				reqIds:  tc.ids,
				docs:    docs,
				path:    t.TempDir(),
			}
			name := e.exportName(date)
			require.Equal(t, "anytype-"+tc.want+"-2026-09-09-163500.123", name)
			for _, zip := range []bool{false, true} {
				e.zip = zip
				wr, err := e.getWriter(name)
				require.NoError(t, err)
				require.NoError(t, wr.Close())
				if zip {
					path, _, err := e.renameZipArchive(wr, name, 1)
					require.NoError(t, err)
					assert.Equal(t, name+".zip", filepath.Base(path))
					assert.FileExists(t, path)
				} else {
					assert.Equal(t, name, filepath.Base(wr.Path()))
					assert.DirExists(t, wr.Path())
				}
			}
		})
	}
}
