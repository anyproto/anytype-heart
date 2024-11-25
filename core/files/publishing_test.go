package files

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublishingAdd(t *testing.T) {

}

func TestPublishingGetVariantInfo(t *testing.T) {

}

func testPublishingAdd(t *testing.T, fx *fixture) *AddResult {
	lastModifiedDate := time.Now()
	buf := strings.NewReader(testFileContent)
	opts := []AddOption{
		WithName(testFileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(buf),
	}
	got, err := fx.FileAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)
	got.Commit()

	fx.addFileObjectToStore(t, got)

	return got
}
