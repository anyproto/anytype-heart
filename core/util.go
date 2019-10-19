package core

import (
	"math/rand"
	"path/filepath"
	"time"
)

const ipfsUrlScheme = "ipfs://"

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

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

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
