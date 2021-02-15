package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
)

type MarketplaceType struct {
	*Set
}

func NewMarketplaceType(ms meta.Service, dbCtrl database.Ctrl) *MarketplaceType {
	return &MarketplaceType{Set: NewSet(ms, dbCtrl)}
}

type MarketplaceRelation struct {
	*Set
}

func NewMarketplaceRelation(ms meta.Service, dbCtrl database.Ctrl) *MarketplaceRelation {
	return &MarketplaceRelation{Set: NewSet(ms, dbCtrl)}
}
