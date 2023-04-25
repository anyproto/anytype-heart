package clientserver

import (
	"encoding/binary"
	"github.com/dgraph-io/badger/v3"
)

const portKey = "drpc/server/port"

type portStorage struct {
	db *badger.DB
}

func (p *portStorage) getPort() (port int, err error) {
	err = p.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(portKey))
		if err != nil {
			return err
		}
		var value []byte
		value, err = item.ValueCopy(value)
		if err != nil {
			return err
		}
		port = int(binary.LittleEndian.Uint16(value))
		return nil
	})
	return
}

func (p *portStorage) setPort(port int) (err error) {
	return p.db.Update(func(txn *badger.Txn) error {
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, uint16(port))
		return txn.Set([]byte(portKey), buf)
	})
}
