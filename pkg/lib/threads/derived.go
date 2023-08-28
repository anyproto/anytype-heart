package threads

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

type DerivedSmartblockIds struct {
	AccountOld      string
	Workspace       string
	Profile         string
	Home            string
	Archive         string
	Widgets         string
	SystemTypes     map[bundle.TypeKey]string
	SystemRelations map[bundle.RelationKey]string
}

func (d DerivedSmartblockIds) IsFilled() bool {
	return d.Workspace != "" && d.Profile != "" && d.Home != "" && d.Archive != "" && d.Widgets != ""
}

func (d DerivedSmartblockIds) IsAccount(id string) bool {
	return id == d.Workspace || id == d.AccountOld
}

func (d DerivedSmartblockIds) HasID(sbt smartblock.SmartBlockType) bool {
	switch sbt {
	case smartblock.SmartBlockTypeWorkspace:
		return d.Workspace != ""
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
		d.Workspace = id
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
