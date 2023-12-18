package objecttype

import (
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("update-last-used-date")

func UpdateLastUsedDate(spc smartblock.Space, store objectstore.ObjectStore, keys []domain.TypeKey) {
	for _, key := range keys {
		uk, err := domain.UnmarshalUniqueKey(key.URL())
		if err != nil {
			log.Errorf("failed to unmarshall type key '%s': %w", key.String(), err)
			continue
		}
		details, err := store.GetObjectByUniqueKey(spc.Id(), uk)
		if err != nil {
			log.Errorf("failed to get details of type object '%s': %w", key.String(), err)
			continue
		}
		id := pbtypes.GetString(details.Details, bundle.RelationKeyId.String())
		if id == "" {
			log.Errorf("failed to get id from details of type object '%s': %w", key.String(), err)
			continue
		}
		if err = spc.Do(id, func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			st.SetLocalDetail(bundle.RelationKeyLastUsedDate.String(), pbtypes.Int64(time.Now().Unix()))
			return sb.Apply(st)
		}); err != nil {
			log.Errorf("failed to set lastUsedDate to type object '%s': %w", key.String(), err)
		}
	}
}
