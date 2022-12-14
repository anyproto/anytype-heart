package editor

import (
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type ThreadDB struct {
	smartblock.SmartBlock
	migrator AccountMigrator
}

type AccountMigrator interface {
	MigrateMany(threadInfos []threads.ThreadInfo) (int, error)
}

func NewThreadDB() *ThreadDB {
	return &ThreadDB{SmartBlock: smartblock.New()}
}

func (p *ThreadDB) Init(ctx *smartblock.InitContext) (err error) {
	p.migrator = app.MustComponent[AccountMigrator](ctx.App)

	if ctx.Source.Type() != model.SmartBlockType_AccountOld {
		return fmt.Errorf("source type should be a workspace or an old account")
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	p.AddHook(p.updateObjects, smartblock.HookAfterApply)
	return nil
}

func (p *ThreadDB) updateObjects(info smartblock.ApplyInfo) error {
	objects := p.workspaceObjectsFromState(info.State)
	log.Debugf("threadDB migrate %d objects", len(objects))
	migrated, err := p.migrator.MigrateMany(objects)
	if err != nil {
		log.Errorf("failed migrating many objects: %s", err.Error())
	} else {
		log.Infof("migrated %d threads", migrated)
	}
	return nil
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
