package wrapper

// IdempotencyKeyHeader is the C8 header every mutation may carry.
const IdempotencyKeyHeader = "Idempotency-Key"

// MaxIdempotencyKeyLen bounds an Idempotency-Key: the server's replay store
// is keyed on the key string, so an unbounded key is an unbounded entry.
// The server refuses longer ones with the same rule (core/api/v2's
// ValidIdempotencyKey); this copy refuses them before a call is sent.
const MaxIdempotencyKeyLen = 255

// ValidIdempotencyKey reports whether key is 1..255 visible ASCII
// characters (0x21–0x7E).
func ValidIdempotencyKey(key string) bool {
	if key == "" || len(key) > MaxIdempotencyKeyLen {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x21 || key[i] > 0x7e {
			return false
		}
	}
	return true
}
