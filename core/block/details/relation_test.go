package details

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const spaceId = "spaceId"

func relationObject(key domain.RelationKey, format model.RelationFormat) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             pbtypes.String(key.URL()),
		bundle.RelationKeySpaceId:        pbtypes.String(spaceId),
		bundle.RelationKeyLayout:         pbtypes.Float64(float64(model.ObjectType_relation)),
		bundle.RelationKeyRelationKey:    pbtypes.String(key.String()),
		bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(format)),
	}
}

func TestService_ListRelationsWithValue(t *testing.T) {
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, []objectstore.TestObject{
		// relations
		relationObject(bundle.RelationKeyLastModifiedDate, model.RelationFormat_date),
		relationObject(bundle.RelationKeyAddedDate, model.RelationFormat_date),
		relationObject(bundle.RelationKeyCreatedDate, model.RelationFormat_date),
		relationObject(bundle.RelationKeyLinks, model.RelationFormat_object),
		relationObject(bundle.RelationKeyName, model.RelationFormat_longtext),
		relationObject(bundle.RelationKeyIsHidden, model.RelationFormat_checkbox),
		relationObject(bundle.RelationKeyIsFavorite, model.RelationFormat_checkbox),
		relationObject("daysTillSummer", model.RelationFormat_number),
		relationObject(bundle.RelationKeyCoverX, model.RelationFormat_number),
		{
			bundle.RelationKeyId:               pbtypes.String("obj1"),
			bundle.RelationKeySpaceId:          pbtypes.String(spaceId),
			bundle.RelationKeyCreatedDate:      pbtypes.Int64(time.Now().Add(-5 * time.Minute).Unix()),
			bundle.RelationKeyAddedDate:        pbtypes.Int64(time.Now().Add(-3 * time.Minute).Unix()),
			bundle.RelationKeyLastModifiedDate: pbtypes.Int64(time.Now().Add(-1 * time.Minute).Unix()),
			bundle.RelationKeyIsFavorite:       pbtypes.Bool(true),
			"daysTillSummer":                   pbtypes.Int64(300),
			bundle.RelationKeyLinks:            pbtypes.StringList([]string{"obj2", "obj3"}),
		},
		{
			bundle.RelationKeyId:               pbtypes.String("obj2"),
			bundle.RelationKeySpaceId:          pbtypes.String(spaceId),
			bundle.RelationKeyName:             pbtypes.String(addr.TimeToID(time.Now())),
			bundle.RelationKeyCreatedDate:      pbtypes.Int64(time.Now().Add(-24*time.Hour - 5*time.Minute).Unix()),
			bundle.RelationKeyAddedDate:        pbtypes.Int64(time.Now().Add(-24*time.Hour - 3*time.Minute).Unix()),
			bundle.RelationKeyLastModifiedDate: pbtypes.Int64(time.Now().Add(-1 * time.Minute).Unix()),
			bundle.RelationKeyCoverX:           pbtypes.Int64(300),
		},
		{
			bundle.RelationKeyId:               pbtypes.String("obj3"),
			bundle.RelationKeySpaceId:          pbtypes.String(spaceId),
			bundle.RelationKeyIsHidden:         pbtypes.Bool(true),
			bundle.RelationKeyCreatedDate:      pbtypes.Int64(time.Now().Add(-3 * time.Minute).Unix()),
			bundle.RelationKeyLastModifiedDate: pbtypes.Int64(time.Now().Unix()),
			bundle.RelationKeyIsFavorite:       pbtypes.Bool(true),
			bundle.RelationKeyCoverX:           pbtypes.Int64(300),
		},
	})

	bs := service{store: store}

	for _, tc := range []struct {
		name             string
		value            *types.Value
		expectedKeys     []string
		expectedCounters []int64
	}{
		{
			"date object - today",
			pbtypes.String(addr.TimeToID(time.Now())),
			[]string{bundle.RelationKeyAddedDate.String(), bundle.RelationKeyCreatedDate.String(), bundle.RelationKeyLastModifiedDate.String(), bundle.RelationKeyName.String()},
			[]int64{1, 2, 3, 1},
		},
		{
			"date object - yesterday",
			pbtypes.String(addr.TimeToID(time.Now().Add(-24 * time.Hour))),
			[]string{bundle.RelationKeyAddedDate.String(), bundle.RelationKeyCreatedDate.String()},
			[]int64{1, 1},
		},
		{
			"number",
			pbtypes.Int64(300),
			[]string{bundle.RelationKeyCoverX.String(), "daysTillSummer"},
			[]int64{2, 1},
		},
		{
			"bool",
			pbtypes.Bool(true),
			[]string{bundle.RelationKeyIsFavorite.String(), bundle.RelationKeyIsHidden.String()},
			[]int64{2, 1},
		},
		{
			"string list",
			pbtypes.StringList([]string{"obj2", "obj3"}),
			[]string{bundle.RelationKeyLinks.String()},
			[]int64{1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys, counters, err := bs.ListRelationsWithValue(spaceId, tc.value)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedKeys, keys)
			assert.Equal(t, tc.expectedCounters, counters)
		})
	}
}
