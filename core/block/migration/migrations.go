package migration

import (
	"sort"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
)

type Migrator interface {
	CreationStateMigration(ctx *smartblock.InitContext) Migration
	StateMigrations() Migrations
}

type TempMigrator interface {
	SetTemporaryMigration(migration func(s *state.State) error)
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

// Compose returns a migration that runs the parent migration and then the child migration.
// The final version of migration is picked from child migration.
func Compose(parent, child Migration) Migration {
	return Migration{
		Version: child.Version,
		Proc: func(s *state.State) {
			parent.Proc(s)
			child.Proc(s)
		},
	}
}

func RunMigrations(sb smartblock.SmartBlock, initCtx *smartblock.InitContext) {
	migrator, ok := sb.(Migrator)
	if !ok {
		return
	}

	if initCtx.IsNewObject {
		def := migrator.CreationStateMigration(initCtx)
		if initCtx.State.MigrationVersion() < def.Version {
			def.Proc(initCtx.State)
			initCtx.State.SetMigrationVersion(def.Version)
		}
	}

	// migs := migrator.StateMigrations()
	// for _, m := range migs.Migrations {
	// 	if m.Version > initCtx.State.MigrationVersion() {
	// 		m.Proc(initCtx.State)
	// 		initCtx.State.SetMigrationVersion(m.Version)
	// 	}
	// }
}
