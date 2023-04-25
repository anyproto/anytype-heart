package migration

import (
	"sort"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
)

type Migrator interface {
	CreationStateMigration(ctx *smartblock.InitContext) Migration
	StateMigrations() Migrations
}

type Migration struct {
	Version uint32
	Proc    func(s *state.State)
}

type Migrations struct {
	Migrations []Migration
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
	return res
}

func RunMigrations(sb smartblock.SmartBlock, initCtx *smartblock.InitContext) error {
	migrator, ok := sb.(Migrator)
	if !ok {
		return nil
	}

	if initCtx.IsNewObject {
		def := migrator.CreationStateMigration(initCtx)
		if initCtx.State.MigrationVersion() < def.Version {
			def.Proc(initCtx.State)
			initCtx.State.SetMigrationVersion(def.Version)
		}
	}

	migs := migrator.StateMigrations()
	for _, m := range migs.Migrations {
		if m.Version > initCtx.State.MigrationVersion() {
			m.Proc(initCtx.State)
			initCtx.State.SetMigrationVersion(m.Version)
		}
	}
	return sb.Apply(initCtx.State, smartblock.NoHistory, smartblock.NoEvent, smartblock.NoRestrictions, smartblock.SkipIfNoChanges)
}
