package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pb"
)

func migrateFilesToObjects(sb smartblock.SmartBlock, fileObjectService fileobject.Service) func(s *state.State) {
	return func(s *state.State) {
		keys := sb.GetAndUnsetFileKeys()
		converted := make([]*pb.ChangeFileKeys, 0, len(keys))
		for _, k := range keys {
			val := k
			converted = append(converted, &val)
		}
		fileObjectService.MigrateBlocks(s, sb.Space(), converted)
	}
}
