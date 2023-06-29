package debug

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anyproto/anytype-heart/core/debug/treearchive"
	"github.com/stretchr/testify/require"
)

func TestBuildFast(t *testing.T) {
	// Specify the directory you want to iterate
	dir := "./testdata"

	// Read the directory
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read dir: %s", err)
	}

	// Iterate over the files
	for _, file := range files {
		t.Run(file.Name(), func(t *testing.T) {
			filePath := filepath.Join(dir, file.Name())

			// open the file
			f, err := os.Open(filePath)
			if err != nil {
				t.Fatalf("Failed to open file: %s", err)
			}
			defer f.Close()

			testBuildFast(t, filePath)
		})
	}
}

func testBuildFast(b *testing.T, filepath string) {
	// todo: replace with less heavy tree
	archive, err := treearchive.Open(filepath)
	if err != nil {
		require.NoError(b, err)
	}
	defer archive.Close()

	importer := treearchive.NewTreeImporter(archive.ListStorage(), archive.TreeStorage())

	err = importer.Import(false, "")
	if err != nil {
		log.Fatal("can't import the tree", err)
	}

	start := time.Now()
	s, err := importer.State(false)
	if err != nil {
		log.Fatal("can't build state:", err)
	}
	b.Logf("fast build took %s", time.Since(start))

	importer2 := treearchive.NewTreeImporter(archive.ListStorage(), archive.TreeStorage())

	err = importer2.Import(false, "")
	if err != nil {
		log.Fatal("can't import the tree", err)
	}

	s2, err := importer2.State(true)
	if err != nil {
		log.Fatal("can't build state:", err)
	}
	b.Logf("slow build took %s", time.Since(start))

	require.Equal(b, s.StringDebug(), s2.StringDebug())

}
