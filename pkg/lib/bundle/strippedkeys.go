package bundle

import "github.com/anyproto/anytype-heart/core/domain"

// DefaultStrippedKeys are high-churn local relation keys excluded from
// subscription events by default. They reach the client only when explicitly
// requested via the subscription's keys field. Smartblock dependent
// subscriptions (which have no keys field) never carry them; the open object
// itself is exempt and keeps delivering them through LocalRelationsKeys.
var DefaultStrippedKeys = []domain.RelationKey{
	RelationKeySyncStatus,
	RelationKeySyncError,
	RelationKeySyncDate,
	RelationKeyLastUsedDate,
	RelationKeyLastOpenedDate,
}

var defaultStrippedKeysSet = func() map[domain.RelationKey]struct{} {
	m := make(map[domain.RelationKey]struct{}, len(DefaultStrippedKeys))
	for _, k := range DefaultStrippedKeys {
		m[k] = struct{}{}
	}
	return m
}()

// IsDefaultStrippedKey reports whether a key is stripped from subscription
// events unless explicitly requested via the keys field.
func IsDefaultStrippedKey(k domain.RelationKey) bool {
	_, ok := defaultStrippedKeysSet[k]
	return ok
}
