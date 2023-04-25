package converter

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

var log = logging.Logger("import")

func GetSourceDetail(fileName, importPath string) string {
	var source bytes.Buffer
	source.WriteString(strings.TrimPrefix(filepath.Ext(fileName), "."))
	source.WriteString(":")
	source.WriteString(importPath)
	source.WriteRune(filepath.Separator)
	source.WriteString(fileName)
	return source.String()
}

func GetDetails(name string) *types.Struct {
	var title string

	if title == "" {
		title = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	}

	fields := map[string]*types.Value{
		bundle.RelationKeyName.String():   pbtypes.String(title),
		bundle.RelationKeySource.String(): pbtypes.String(name),
	}
	return &types.Struct{Fields: fields}
}

func AddObjectToSnapshot(snapshots *Snapshot, targetID []string) {
	snapshots.Snapshot.Data.Collections = &types.Struct{
		Fields: map[string]*types.Value{smartblock.CollectionStoreKey: pbtypes.StringList(targetID)},
	}
}
