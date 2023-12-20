package objecttype

import (
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const maxInstallationTime = 5 * time.Minute

var (
	log = logging.Logger("update-last-used-date")

	// clients sort lists of object types on descending lastUsedDate value
	lastUsedDateDecrement = map[string]time.Duration{
		bundle.TypeKeyNote.BundledURL():       1 * maxInstallationTime,
		bundle.TypeKeyPage.BundledURL():       2 * maxInstallationTime,
		bundle.TypeKeyTask.BundledURL():       3 * maxInstallationTime,
		bundle.TypeKeySet.BundledURL():        4 * maxInstallationTime,
		bundle.TypeKeyCollection.BundledURL(): 5 * maxInstallationTime,
	}
)

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

func isCrucialObjectType(id string) bool {
	return lo.Contains([]string{
		bundle.TypeKeyNote.String(),
		bundle.TypeKeyPage.String(),
		bundle.TypeKeyTask.String(),
		bundle.TypeKeySet.String(),
		bundle.TypeKeyCollection.String(),
	}, strings.TrimPrefix(id, addr.BundledObjectTypeURLPrefix))
}

func SetLastUsedDateForCrucialType(id string, details *types.Struct) {
	if !isCrucialObjectType(id) {
		return
	}
	if details == nil || details.Fields == nil {
		return
	}
	// we do this trick to order crucial Anytype object types by last date
	lastUsed := time.Now().Add(-1 * lastUsedDateDecrement[id]).Unix()
	details.Fields[bundle.RelationKeyLastUsedDate.String()] = pbtypes.Int64(lastUsed)
}
