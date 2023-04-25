package migration

import (
	"sort"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
)

type Migrator interface {
	DefaultState(ctx *smartblock.InitContext) Migration
	StateMigrations() Migrations
}

type Migration struct {
	Version uint32
	Proc    func(s *state.State)
}

type Migrations struct {
	LastVersion uint32
	Migrations  []Migration
}

func (m Migrations) Len() int {
	return len(m.Migrations)
}

func (m Migrations) Less(i, j int) bool {
	return m.Migrations[i].Version < m.Migrations[j].Version
}

func (m Migrations) Swap(i, j int) {
	m.Migrations[i], m.Migrations[j] = m.Migrations[j], m.Migrations[i]
}

func MakeMigrations(migrations []Migration) Migrations {
	res := Migrations{
		Migrations: migrations,
	}
	sort.Sort(res)
	for i := 0; i < len(res.Migrations)-1; i++ {
		if res.Migrations[i].Version == res.Migrations[i+1].Version {
			panic("two migrations have the same version")
		}
	}
	if len(migrations) > 0 {
		res.LastVersion = migrations[len(migrations)-1].Version
	}

	return res
}
