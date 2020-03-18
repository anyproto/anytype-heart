package vclock

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
)

// Condition constants define how to compare a vector clock against another,
// and may be ORed together when being provided to the Compare method.
type Condition int

//Constants define compairison conditions between pairs of vector
//clocks
const (
	Equal Condition = 1 << iota
	Ancestor
	Descendant
	Concurrent
)

type VClock struct {
	mutex *sync.RWMutex
	m     map[string]uint64
}

var Undef = VClock{}

// New returns a new vector clock
// VClock is thread safe
func New() VClock {
	return VClock{mutex: &sync.RWMutex{}, m: make(map[string]uint64)}
}

func NewFromMap(m map[string]uint64) VClock {
	return VClock{mutex: &sync.RWMutex{}, m: m}
}

//Merge takes the max of all clock values in other and updates the
//values of the callee
func (vc VClock) Merge(other VClock) {
	vc.mutex.Lock()
	defer vc.mutex.Unlock()
	other.mutex.RLock()
	defer other.mutex.RUnlock()
	for id := range other.m {
		if vc.m[id] < other.m[id] {
			vc.m[id] = other.m[id]
		}
	}
}

//MarshalBinary returns an encoded vector clock
func (vc VClock) MarshalBinary() ([]byte, error) {
	vc.mutex.RLock()
	defer vc.mutex.RUnlock()

	b := new(bytes.Buffer)
	enc := gob.NewEncoder(b)
	err := enc.Encode(vc.m)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

//UnmarshalBinary decodes a vector clock
func UnmarshalBinary(data []byte) (vc VClock, err error) {
	b := new(bytes.Buffer)
	b.Write(data)
	clock := New()
	dec := gob.NewDecoder(b)
	err = dec.Decode(&clock)
	return clock, err
}

func (vc VClock) String() string {
	vc.mutex.RLock()
	defer vc.mutex.RUnlock()

	ids := make([]string, 0, len(vc.m))
	i := 0
	for id := range vc.m {
		ids[i] = id
		i++
	}

	sort.Strings(ids)
	var buffer bytes.Buffer
	buffer.WriteString("{")
	for i := range ids {
		buffer.WriteString(fmt.Sprintf("\"%s\":%d", ids[i], vc.m[ids[i]]))
		if i+1 < len(ids) {
			buffer.WriteString(", ")
		}
	}

	buffer.WriteString("}")
	return buffer.String()
}

func (vc VClock) Compare(other VClock, cond Condition) bool {
	vc.mutex.RLock()
	defer vc.mutex.RUnlock()

	other.mutex.RLock()
	defer other.mutex.RUnlock()

	var otherIs Condition
	// Preliminary qualification based on length
	if len(vc.m) > len(other.m) {
		if cond&(Ancestor|Concurrent) == 0 {
			return false
		}
		otherIs = Ancestor
	} else if len(vc.m) < len(other.m) {
		if cond&(Descendant|Concurrent) == 0 {
			return false
		}
		otherIs = Descendant
	} else {
		otherIs = Equal
	}

	//Compare matching items
	for id := range other.m {
		if _, found := vc.m[id]; found {
			if other.m[id] > vc.m[id] {
				switch otherIs {
				case Equal:
					if cond&Descendant == 0 {
						return false
					}
					otherIs = Descendant
					break
				case Ancestor:
					return cond&Concurrent != 0
				}
			} else if other.m[id] < vc.m[id] {
				switch otherIs {
				case Equal:
					if cond&Ancestor == 0 {
						return false
					}
					otherIs = Ancestor
					break
				case Descendant:
					return cond&Concurrent != 0
				}
			}
		} else {
			if otherIs == Equal {
				return cond&Concurrent != 0
			} else if (len(other.m) - len(vc.m) - 1) < 0 {
				return cond&Concurrent != 0
			}
		}
	}
	return cond&otherIs != 0
}

func (vc VClock) Copy() VClock {
	vc.mutex.RLock()
	defer vc.mutex.RUnlock()

	cp := make(map[string]uint64, len(vc.m))
	for key, value := range vc.m {
		cp[key] = value
	}
	return VClock{mutex: &sync.RWMutex{}, m: cp}
}

func (vc VClock) Map() map[string]uint64 {
	vc.mutex.RLock()
	defer vc.mutex.RUnlock()

	cp := make(map[string]uint64, len(vc.m))
	for key, value := range vc.m {
		cp[key] = value
	}

	return cp
}


func (vc VClock) Hash() string {
	var (
		sum     [32]byte
		hashArr [512]byte
		hashBuf = hashArr[:0]
		keysArr [256]byte
		keysBuf = keysArr[:0]
	)
	keys := make([]string, 0, len(vc.m))
	for k := range vc.m {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	for _, k := range keys {
		keysBuf = keysBuf[:0]
		keysBuf = append(keysBuf, []byte(k)...)
		keysBuf = append(keysBuf, []byte("=")...)
		hashBuf = hashBuf[:0]
		hashBuf = append(hashBuf, sum[:]...)
		hashBuf = append(hashBuf, keysBuf...)
		v := vc.m[k]
		hashBuf = append(hashBuf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24), byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
		sum = sha256.Sum256(hashBuf)
	}
	return hex.EncodeToString(sum[:])
}
