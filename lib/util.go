package main

import (
	"math/rand"
	"path/filepath"
	"time"

	"github.com/gogo/protobuf/proto"
	multihash "github.com/multiformats/go-multihash"
	"github.com/sirupsen/logrus"
)

const ipfsUrlScheme = "ipfs://"

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func init() {
	rand.Seed(time.Now().UnixNano())
}

func ipfsFileURL(hash multihash.Multihash, originalFileName string) string {
	url := ipfsUrlScheme + hash.B58String()
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

func Marshal(msg proto.Message) []byte {
	b, err := proto.Marshal(msg)
	if err != nil {
		logrus.Errorf("failed to marshal: %s", err.Error())
		return nil
	}

	return b
}
