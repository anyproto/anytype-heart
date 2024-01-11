package main

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

func main() {
	bd, err := badger.Open(badger.DefaultOptions("tmp"))
	if err != nil {
		panic(err)
	}
	i := 1
	for {
		i++
		err = bd.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(fmt.Sprintf("key%d", i)), []byte(fmt.Sprintf("key%d", i)))
		})
		if err != nil {
			panic(err)
		}
	}
}
