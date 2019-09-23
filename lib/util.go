package main

import (
	"math/rand"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func init() {
	rand.Seed(time.Now().UnixNano())
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
