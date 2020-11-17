package symmetric

import (
	"crypto/rand"
	"fmt"
	"io"

	mbase "github.com/multiformats/go-multibase"
)

type EncryptorDecryptor interface {
	EncryptReader(r io.Reader) (io.Reader, error)
	DecryptReader(r io.ReadSeeker) (ReadSeekCloser, error)
}

type ReadSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

const (
	// KeyBytes is the length of AES key.
	KeyBytes = 32
)

// Key is a wrapper for a symmetric key.
type Key []byte

// NewRandom returns a random key.
func NewRandom() (Key, error) {
	raw := make([]byte, KeyBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	return Key(raw), nil
}

// FromBytes returns a key by decoding bytes.
func FromBytes(k []byte) (Key, error) {
	if len(k) != KeyBytes {
		return nil, fmt.Errorf("invalid key")
	}
	return Key(k), nil
}

// FromString returns a key by decoding a base32-encoded string.
func FromString(k string) (Key, error) {
	_, b, err := mbase.Decode(k)
	if err != nil {
		return nil, err
	}
	return FromBytes(b)
}

// Bytes returns Raw key bytes.
func (k Key) Bytes() []byte {
	return k
}

// MarshalBinary implements BinaryMarshaler.
func (k Key) MarshalBinary() ([]byte, error) {
	return k, nil
}

// String returns the base32-encoded string representation of Raw key bytes.
func (k Key) String() string {
	str, err := mbase.Encode(mbase.Base32, k)
	if err != nil {
		panic("should not error with hardcoded mbase: " + err.Error())
	}
	return str
}
