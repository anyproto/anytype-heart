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

	fi := givenFileInfo()

	doc := marshalFileInfo(arena, fi)

	got, err := unmarshalFileInfo(doc)

	require.NoError(t, err)
	assert.Equal(t, fi, got)
}

func givenFileInfo() FileInfo {

	testFileId := domain.FileId("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")
	testCid2 := cid.MustParse("bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi")
	testCid3 := cid.MustParse("bafybeie5gq4jxvzmsym6hjlwxej4rwdoxt7wadqvmmwbqi7r27irfvnfza")

	return FileInfo{
		FileId:              testFileId,
		SpaceId:             "space1",
		ObjectId:            "object1",
		State:               FileStateUploading,
		ScheduledAt:         time.Date(2021, time.December, 31, 12, 55, 12, 0, time.UTC),
		Variants:            []domain.FileId{"variant1", "variant2"},
		AddedByUser:         true,
		Imported:            true,
		BytesToUploadOrBind: 123,
		CidsToBind: map[cid.Cid]struct{}{
			cid.MustParse(testFileId.String()): {},
			testCid2:                           {},
			testCid3:                           {},
		},
		CidsToUpload: map[cid.Cid]struct{}{
			cid.MustParse(testFileId.String()): {},
			testCid2:                           {},
			testCid3:                           {},
		},
	}
}
