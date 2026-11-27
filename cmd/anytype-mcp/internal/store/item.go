package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ItemVersion is the item format this binary writes and reads.
const ItemVersion = 1

// MaxKeyBytes bounds the key, and MaxItemBytes the whole stored item: a
// longer file is not an item.
const (
	MaxKeyBytes  = 1024
	MaxItemBytes = 4 << 10
)

var (
	connectionRe = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	accountRe    = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,256}$`)
)

// ValidConnectionID reports whether id may name a connection.
func ValidConnectionID(id string) bool { return connectionRe.MatchString(id) }

// ValidTier reports whether t is a tier heart serves.
func ValidTier(t string) bool { return t == "small" || t == "large" || t == "full" }

// Item is the connection: everything serve needs, in one file.
type Item struct {
	V       int    `json:"v"`
	Rev     int64  `json:"rev"`
	Key     string `json:"key"`
	Account string `json:"account"`
	Port    int    `json:"port"`
	Tier    string `json:"tier"`
}

// ErrInvalidItem is a stored value that is not an item this binary wrote.
var ErrInvalidItem = errors.New("the stored item is not valid")

// Validate checks every field.
func (it Item) Validate() error {
	switch {
	case it.V != ItemVersion:
		return fmt.Errorf("unsupported item version %d", it.V)
	case it.Rev < 1:
		return errors.New("rev must be at least 1")
	case it.Port < 1 || it.Port > 65535:
		return errors.New("port must be between 1 and 65535")
	case !ValidTier(it.Tier):
		return errors.New("tier must be small, large or full")
	case !accountRe.MatchString(it.Account):
		return errors.New("account is missing or malformed")
	}
	return validKey(it.Key)
}

func validKey(key string) error {
	if key == "" {
		return errors.New("key is missing")
	}
	if len(key) > MaxKeyBytes {
		return fmt.Errorf("key is longer than %d bytes", MaxKeyBytes)
	}
	// the key travels in an HTTP header: visible ASCII only
	if strings.IndexFunc(key, func(r rune) bool { return r <= 0x20 || r >= 0x7f }) >= 0 {
		return errors.New("key has characters a header can't carry")
	}
	return nil
}

// Encode is the stored form.
func (it Item) Encode() ([]byte, error) {
	if err := it.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(it)
}

// DecodeItem parses a stored value: exactly one valid item and nothing
// after it. Anything else is ErrInvalidItem; the message never carries the
// value.
func DecodeItem(raw []byte) (Item, error) {
	var it Item
	if len(raw) > MaxItemBytes {
		return Item{}, ErrInvalidItem
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&it); err != nil {
		return Item{}, ErrInvalidItem
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Item{}, ErrInvalidItem
	}
	if err := it.Validate(); err != nil {
		return Item{}, fmt.Errorf("%w: %w", ErrInvalidItem, err)
	}
	return it, nil
}

// Fingerprint is the registry's key fingerprint: the first 16 hex digits
// of sha256(key).
func Fingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:16]
}

// Fingerprint of the item's key.
func (it Item) Fingerprint() string { return Fingerprint(it.Key) }

// Stamp is the precondition form of an item's state: <fp>@<rev>.
func (it Item) Stamp() string { return fmt.Sprintf("%s@%d", it.Fingerprint(), it.Rev) }
