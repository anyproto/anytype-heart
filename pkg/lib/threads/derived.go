package threads

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

var (
	PersonalSpaceTypes = []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeHome,
		smartblock.SmartBlockTypeArchive,
		smartblock.SmartBlockTypeWidget,
		smartblock.SmartBlockTypeWorkspace,
		smartblock.SmartBlockTypeProfilePage,
	}
	SpaceTypes = []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeHome,
		smartblock.SmartBlockTypeArchive,
		smartblock.SmartBlockTypeWidget,
		smartblock.SmartBlockTypeWorkspace,
	}
)

type DerivedSmartblockIds struct {
	Workspace       string
	Profile         string
	Home            string
	Archive         string
	Widgets         string
	SystemTypes     map[domain.TypeKey]string
	SystemRelations map[domain.RelationKey]string
	SpaceChat       string
}

func (d DerivedSmartblockIds) IDs() []string {
	allIds := []string{
		d.Workspace,
		d.Home,
		d.Archive,
		d.Widgets,
	}
	// todo: should it include system types/relations?
	if d.Profile != "" {
		allIds = append(allIds, d.Profile)
	}
	return allIds
}

func (d DerivedSmartblockIds) IDsWithSystemTypesAndRelations() []string {
	allIds := d.IDs()
	for _, id := range d.SystemTypes {
		allIds = append(allIds, id)
	}
	for _, id := range d.SystemRelations {
		allIds = append(allIds, id)
	}
	return allIds
}

func (d DerivedSmartblockIds) IsFilled() bool {
	for _, id := range d.IDs() {
		if id == "" {
			return false
		}
	}
	return true
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
