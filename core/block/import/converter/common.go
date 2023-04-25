package converter

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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

func UpdateLinksToObjects(st *state.State, oldIDtoNew map[string]string, pageID string) error {
	return st.Iterate(func(bl simple.Block) (isContinue bool) {
		switch a := bl.(type) {
		case link.Block:
			newTarget := oldIDtoNew[a.Model().GetLink().TargetBlockId]
			if newTarget == "" {
				// maybe we should panic here?
				log.With("object", st.RootId()).Errorf("cant find target id for link: %s", a.Model().GetLink().TargetBlockId)
				return true
			}

			a.Model().GetLink().TargetBlockId = newTarget
			st.Set(simple.New(a.Model()))
		case bookmark.Block:
			newTarget := oldIDtoNew[a.Model().GetBookmark().TargetObjectId]
			if newTarget == "" {
				// maybe we should panic here?
				log.With("object", pageID).Errorf("cant find target id for bookmark: %s", a.Model().GetBookmark().TargetObjectId)
				return true
			}

			a.Model().GetBookmark().TargetObjectId = newTarget
			st.Set(simple.New(a.Model()))
		case text.Block:
			for i, mark := range a.Model().GetText().GetMarks().GetMarks() {
				if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
					continue
				}
				newTarget := oldIDtoNew[mark.Param]
				if newTarget == "" {
					log.With("object", pageID).Errorf("cant find target id for mention: %s", mark.Param)
					continue
				}

				a.Model().GetText().GetMarks().GetMarks()[i].Param = newTarget
			}
			st.Set(simple.New(a.Model()))
		}
		return true
	})
}
