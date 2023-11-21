package objecttype

import (
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func UpdateLastUsedDate(spc smartblock.Space, store objectstore.ObjectStore, keys []domain.TypeKey) error {
	for _, key := range keys {
		uk, err := domain.UnmarshalUniqueKey(key.URL())
		if err != nil {
			return fmt.Errorf("failed to unmarshall type key '%s': %w", key.String(), err)
		}
		details, err := store.GetObjectByUniqueKey(spc.Id(), uk)
		if err != nil {
			return fmt.Errorf("failed to get details of type object '%s': %w", key.String(), err)
		}
		id := pbtypes.GetString(details.Details, bundle.RelationKeyId.String())
		if id == "" {
			return fmt.Errorf("failed to get id from details of type object '%s': %w", key.String(), err)
		}
		if err = spc.Do(id, func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			st.SetLocalDetail(bundle.RelationKeyLastUsedDate.String(), pbtypes.Int64(time.Now().Unix()))
			return sb.Apply(st)
		}); err != nil {
			return fmt.Errorf("failed to set lastUsedDate to type object '%s': %w", key.String(), err)
		}
	}
	return nil
}
