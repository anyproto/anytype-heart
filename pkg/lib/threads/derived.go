package threads

import (
	"fmt"
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

	threadDerivedIndexMarketplaceType     threadDerivedIndex = 30
	threadDerivedIndexMarketplaceRelation threadDerivedIndex = 31
	threadDerivedIndexMarketplaceTemplate threadDerivedIndex = 32

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
	AccountOld          string
	Account             string
	Profile             string
	Home                string
	Archive             string
	MarketplaceType     string
	MarketplaceRelation string
	MarketplaceTemplate string
	Widgets             string
}

func (d DerivedSmartblockIds) IsAccount(id string) bool {
	return id == d.Account || id == d.AccountOld
}

var ErrAddReplicatorsAttemptsExceeded = fmt.Errorf("add replicatorAddr attempts exceeded")
