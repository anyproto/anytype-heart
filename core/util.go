package core

import (
	"math/rand"
	"path/filepath"
	"time"
)

const ipfsUrlScheme = "ipfs://"

func init() {
	rand.Seed(time.Now().UnixNano())
}

func ipfsFileURL(hash string, originalFileName string) string {
	url := ipfsUrlScheme + hash
	if originalFileName != "" {
		url += "/" + filepath.Base(originalFileName)
	}

	return url
}
