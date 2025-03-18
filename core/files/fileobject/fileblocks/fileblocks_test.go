package fileblocks

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestAddFileBlocks(t *testing.T) {
	id := "some_file"

	for _, tc := range []struct {
		name              string
		details           *domain.Details
		expectedRelations []domain.RelationKey
	}{
		{
			"image",
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:           domain.String("photo.jpeg"),
				bundle.RelationKeyFileMimeType:   domain.String("image/jpeg"),
				bundle.RelationKeyWidthInPixels:  domain.Int64(400),
				bundle.RelationKeyHeightInPixels: domain.Int64(600),
				bundle.RelationKeyAddedDate:      domain.Int64(time.Now().Unix()),
			}),
			[]domain.RelationKey{bundle.RelationKeyDescription, bundle.RelationKeyFileExt, bundle.RelationKeyWidthInPixels, bundle.RelationKeyHeightInPixels, bundle.RelationKeyAddedDate},
		},
		{
			"plain file",
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:        domain.String("txt.txt"),
				bundle.RelationKeySizeInBytes: domain.Int64(24000),
				bundle.RelationKeyOrigin:      domain.Int64(int64(model.ObjectOrigin_dragAndDrop)),
				bundle.RelationKeyAddedDate:   domain.Int64(time.Now().Unix()),
			}),
			[]domain.RelationKey{bundle.RelationKeyDescription, bundle.RelationKeyFileExt, bundle.RelationKeySizeInBytes, bundle.RelationKeyOrigin, bundle.RelationKeyAddedDate},
		},
		{
			"audio",
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:                  domain.String("song.mp3"),
				bundle.RelationKeyFileMimeType:          domain.String("audio/mp3"),
				bundle.RelationKeySizeInBytes:           domain.Int64(2400000),
				bundle.RelationKeyAudioAlbum:            domain.String("Never mind"),
				bundle.RelationKeyAudioAlbumTrackNumber: domain.Int64(13),
				bundle.RelationKeyOrigin:                domain.Int64(int64(model.ObjectOrigin_clipboard)),
				bundle.RelationKeyImportType:            domain.Int64(2),
			}),
			[]domain.RelationKey{bundle.RelationKeyDescription, bundle.RelationKeyFileExt, bundle.RelationKeySizeInBytes, bundle.RelationKeyAudioAlbum, bundle.RelationKeyAudioAlbumTrackNumber, bundle.RelationKeyOrigin, bundle.RelationKeyImportType},
		},
	} {
		t.Run(fmt.Sprintf("add file blocks: %s", tc.name), func(t *testing.T) {
			// given
			st := state.NewDoc(id, map[string]simple.Block{
				id: simple.New(&model.Block{Id: id}),
			}).NewState()

			// when
			err := AddFileBlocks(st, tc.details, id)

			// then
			assert.NoError(t, err)
			assertBlocks(t, st.Blocks(), tc.expectedRelations)
		})
	}
}

func assertBlocks(t *testing.T, blocks []*model.Block, relations []domain.RelationKey) {
	counter := 0
	var txtFound, fileFound bool
	for _, block := range blocks {
		rb := block.GetRelation()
		if rb != nil {
			assert.Contains(t, relations, domain.RelationKey(rb.Key))
			counter++
			continue
		}

		txt := block.GetText()
		if txt != nil {
			assert.Equal(t, "File Information", txt.GetText())
			txtFound = true
			continue
		}

		file := block.GetFile()
		if file != nil {
			fileFound = true
		}
	}
	assert.Equal(t, counter, len(relations))
	assert.True(t, txtFound)
	assert.True(t, fileFound)
}
