package threads

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

type threadDerivedIndex uint32

const (
	// profile page is publicly accessible as service/read keys derived from account public key
	threadDerivedIndexProfilePage threadDerivedIndex = 0
	threadDerivedIndexHome        threadDerivedIndex = 1
	threadDerivedIndexArchive     threadDerivedIndex = 2
	threadDerivedIndexAccountOld  threadDerivedIndex = 3
	threadDerivedIndexAccount     threadDerivedIndex = 4
	threadDerivedIndexWidgets     threadDerivedIndex = 5

	threadDerivedIndexSetPages threadDerivedIndex = 20 // deprecated

	threadDerivedIndexMarketplaceType     threadDerivedIndex = 30 // deprecated
	threadDerivedIndexMarketplaceRelation threadDerivedIndex = 31 // deprecated
	threadDerivedIndexMarketplaceTemplate threadDerivedIndex = 32 // deprecated

	anytypeThreadSymmetricKeyPathPrefix = "m/SLIP-0021/anytype"
	// TextileAccountPathFormat is a path format used for Anytype keypair
	// derivation as described in SEP-00XX. Use with `fmt.Sprintf` and `DeriveForPath`.
	// m/SLIP-0021/anytype/<predefined_thread_index>/%d/<label>
	anytypeThreadPathFormat = anytypeThreadSymmetricKeyPathPrefix + `/%d/%s`

	anytypeThreadServiceKeySuffix = `service`
	anytypeThreadReadKeySuffix    = `read`
	anytypeThreadIdKeySuffix      = `id`
)

type DerivedSmartblockIds struct {
	AccountOld      string
	Account         string
	Profile         string
	Home            string
	Archive         string
	Widgets         string
	SystemTypes     map[bundle.TypeKey]string
	SystemRelations map[bundle.RelationKey]string
}

func (d DerivedSmartblockIds) IsFilled() bool {
	return d.Account != "" && d.Profile != "" && d.Home != "" && d.Archive != "" && d.Widgets != ""
}

func (d DerivedSmartblockIds) IsAccount(id string) bool {
	return id == d.Account || id == d.AccountOld
}

func (d DerivedSmartblockIds) HasID(sbt smartblock.SmartBlockType) bool {
	switch sbt {
	case smartblock.SmartBlockTypeWorkspace:
		return d.Account != ""
	case smartblock.SmartBlockTypeWidget:
		return d.Widgets != ""
	case smartblock.SmartBlockTypeHome:
		return d.Home != ""
	case smartblock.SmartBlockTypeArchive:
		return d.Archive != ""
	case smartblock.SmartBlockTypeProfilePage:
		return d.Profile != ""
	default:
		panic(fmt.Sprintf("don't know %s", sbt.ToProto().String()))
	}
}

func (d *DerivedSmartblockIds) InsertId(sbt smartblock.SmartBlockType, id string) {
	switch sbt {
	case smartblock.SmartBlockTypeWorkspace:
		d.Account = id
	case smartblock.SmartBlockTypeWidget:
		d.Widgets = id
	case smartblock.SmartBlockTypeHome:
		d.Home = id
	case smartblock.SmartBlockTypeArchive:
		d.Archive = id
	case smartblock.SmartBlockTypeProfilePage:
		d.Profile = id
	default:
		panic(fmt.Sprintf("don't know %s/%s", sbt.ToProto().String(), id))
	}
}
