package filesync

import (
	"testing"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

func TestMarshalUnmarshal(t *testing.T) {
	arena := &anyenc.Arena{}

	testFileId := domain.FileId("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")

	fi := FileInfo{
		FileId:        testFileId,
		SpaceId:       "space1",
		ObjectId:      "object1",
		State:         FileStateUploading,
		AddedAt:       time.Date(2021, time.December, 31, 12, 55, 12, 0, time.UTC),
		HandledAt:     time.Date(2022, time.January, 1, 13, 56, 13, 0, time.UTC),
		Variants:      []domain.FileId{"variant1", "variant2"},
		AddedByUser:   true,
		Imported:      true,
		BytesToUpload: 123,
		CidsToUpload: map[cid.Cid]struct{}{
			cid.MustParse(testFileId.String()): {},
		},
	}

	doc := marshalFileInfo(fi, arena)

	got, err := unmarshalFileInfo(doc)

	require.NoError(t, err)
	assert.Equal(t, fi, got)
}
