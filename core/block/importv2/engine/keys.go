package engine

import "sync"

// KeyTable maps converter-emitted relation/type internal keys onto adopted
// existing keys. It implements resolve.KeyResolver. Safe for concurrent use.
type KeyTable struct {
	mu sync.RWMutex
	m  map[string]string
}

func NewKeyTable() *KeyTable {
	return &KeyTable{m: map[string]string{}}
}

func (k *KeyTable) Set(sourceKey, finalKey string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[sourceKey] = finalKey
}

func (k *KeyTable) FinalKey(sourceKey string) (string, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	finalKey, ok := k.m[sourceKey]
	return finalKey, ok
}
