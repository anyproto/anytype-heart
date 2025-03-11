package csv

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

type mockWriter struct {
	tempDir string
}

func (m *mockWriter) WriteFile(filename string, r io.Reader, lastModifiedDate int64) error {
	dir := filepath.Dir(filename)
	fullPath := filepath.Join(m.tempDir, dir)
	err := os.MkdirAll(fullPath, 0700)
	if err != nil {
		return err
	}
	filename = filepath.Join(m.tempDir, filename)
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = io.Copy(f, r); err != nil {
		return err
	}
	return nil
}

func TestObjectTypeFiles_GetFileOrCreate(t *testing.T) {
	t.Run("create new file", func(t *testing.T) {
		// given
		objectTypeFiles := ObjectTypeFiles{}

		// when
		file, err := objectTypeFiles.GetFileOrCreate("test", nil)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, file)
		assert.Len(t, objectTypeFiles, 1)
		assert.NotNil(t, objectTypeFiles[filepath.Join(objectTypesDirectory, "test.csv")])
	})

	t.Run("get existing file", func(t *testing.T) {
		// given
		objectTypeFiles := ObjectTypeFiles{}

		// when
		_, err := objectTypeFiles.GetFileOrCreate("test", nil)
		assert.NoError(t, err)
		file, _ := objectTypeFiles.GetFileOrCreate("test", nil)

		// then
		assert.NoError(t, err)
		assert.NotNil(t, file)
		assert.Len(t, objectTypeFiles, 1)
		assert.NotNil(t, objectTypeFiles[filepath.Join(objectTypesDirectory, "test.csv")])
	})
}

func TestObjectTypeFiles_Flush(t *testing.T) {
	t.Run("Flush successfully", func(t *testing.T) {
		// given
		objectFiles := ObjectTypeFiles{}
		filePath := filepath.Join(objectTypesDirectory, "test.csv")
		objectFiles[filePath] = &objectType{fileName: filePath, csvRows: [][]string{{"header1", "header2"}}}

		// when
		tempDir := t.TempDir()
		w := &mockWriter{tempDir: tempDir}
		err := objectFiles.Flush(w)

		// then
		assert.NoError(t, err)
		file, err := os.Open(filepath.Join(tempDir, filePath))
		assert.NoError(t, err)
		defer file.Close()
		buffer := bytes.NewBuffer(nil)
		_, err = io.Copy(buffer, file)
		assert.NoError(t, err)
		assert.Equal(t, "header1,header2\n", buffer.String())
	})
	t.Run("Flush with empty data", func(t *testing.T) {
		// given
		objectFiles := ObjectTypeFiles{}
		filePath := filepath.Join(objectTypesDirectory, "test.csv")
		objectFiles[filePath] = &objectType{fileName: filePath, csvRows: [][]string{}}

		// when
		tempDir := t.TempDir()
		w := &mockWriter{tempDir: tempDir}
		err := objectFiles.Flush(w)

		// then
		assert.NoError(t, err)
		_, err = os.Open(filepath.Join(tempDir, objectTypesDirectory, "test.csv"))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestObjectType_WriteRecord(t *testing.T) {
	t.Run("write rows", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyRelationKey: domain.String(bundle.RelationKeyName.String()),
				bundle.RelationKeySpaceId:     domain.String("spaceId"),
				bundle.RelationKeyName:        domain.String("Name"),
				bundle.RelationKeyId:          domain.String("id1"),
			},
			{
				bundle.RelationKeyRelationKey: domain.String(bundle.RelationKeySpaceId.String()),
				bundle.RelationKeySpaceId:     domain.String("spaceId"),
				bundle.RelationKeyName:        domain.String("Space"),
				bundle.RelationKeyId:          domain.String("id2"),
			},
			{
				bundle.RelationKeyRelationKey: domain.String(bundle.RelationKeySourceFilePath.String()),
				bundle.RelationKeySpaceId:     domain.String("spaceId"),
				bundle.RelationKeyName:        domain.String("Source"),
				bundle.RelationKeyId:          domain.String("id3"),
			},
		})
		objType := newObjectType("test.csv", store.SpaceIndex("spaceId"))

		// when
		st := state.NewDoc("root", nil).(*state.State)
		st.SetDetail(bundle.RelationKeyName, domain.String("name"))
		st.SetLocalDetail(bundle.RelationKeySpaceId, domain.String("spaceId"))

		err := objType.WriteRecord(st, "test.csv")

		// then
		assert.NoError(t, err)
		assert.NotEmpty(t, objType.csvRows)
	})
}
