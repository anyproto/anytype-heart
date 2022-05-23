package editor

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

func NewThreadDB(migrator AccountMigrator) *ThreadDB {
	return &ThreadDB{
		SmartBlock: smartblock.New(),
		migrator:   migrator,
	}
}

type AccountMigrator interface {
	MigrateMany(threadInfos []threads.ThreadInfo) (int, error)
}

type ThreadDB struct {
	smartblock.SmartBlock
	migrator AccountMigrator
}

func (p *ThreadDB) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.Source.Type() != model.SmartBlockType_AccountOld {
		return fmt.Errorf("source type should be a workspace or an old account")
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	p.AddHook(p.updateObjects, smartblock.HookAfterApply)
	return smartblock.ObjectApplyTemplate(p, ctx.State,
		template.WithEmpty,
		template.WithTitle,
	)
}

func (p *ThreadDB) updateObjects() {
	st := p.NewState()

	objects := p.workspaceObjectsFromState(st)
	log.Debugf("threadDB migrate %d objects", len(objects))
	migrated, err := p.migrator.MigrateMany(objects)
	if err != nil {
		log.Errorf("failed migrating many objects: %s", err.Error())
	} else {
		log.Infof("migrated %d threads", migrated)
	}
}

func (p *ThreadDB) workspaceObjectsFromState(st *state.State) []threads.ThreadInfo {
	workspaceCollection := st.GetCollection(source.WorkspaceCollection)
	if workspaceCollection == nil || workspaceCollection.Fields == nil {
		return nil
	}
	objects := make([]threads.ThreadInfo, 0, len(workspaceCollection.Fields))
	for _, value := range workspaceCollection.Fields {
		if value == nil {
			continue
		}
		objects = append(objects, p.threadInfoFromThreadDBPB(value))
	}

	return objects
}

func (p *ThreadDB) threadInfoFromThreadDBPB(val *types.Value) threads.ThreadInfo {
	fields := val.Kind.(*types.Value_StructValue).StructValue
	return threads.ThreadInfo{
		ID:    pbtypes.GetString(fields, collectionKeyId),
		Key:   pbtypes.GetString(fields, collectionKeyKey),
		Addrs: pbtypes.GetStringListValue(fields.Fields[collectionKeyAddrs]),
	}
}
