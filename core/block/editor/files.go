package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewFiles(m meta.Service) *Files {
	return &Files{
		SmartBlock: smartblock.New(m, ""),
	}
}

type Files struct {
	smartblock.SmartBlock
}

func (p *Files) Init(s source.Source, allowEmpty bool, _ []string) (err error) {
	if !s.Virtual() {
		return fmt.Errorf("source should be a virtual file")
	}

	if err = p.SmartBlock.Init(s, true, nil); err != nil {
		return
	}
	doc, err := s.ReadDoc(nil, true)
	if err != nil {
		return err
	}
	d := doc.Details()

	fileBlock := &model.Block{
		Id: "file",
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:    pbtypes.GetString(d, "name"),
				Mime:    pbtypes.GetString(d, "fileMimeType"),
				Hash:    p.Id(),
				Size_:   int64(pbtypes.GetFloat64(d, "sizeBytes")),
				State:   model.BlockContentFile_Done,
				AddedAt: int64(pbtypes.GetFloat64(d, "addedDate")),
			},
		}}

	return template.ApplyTemplate(p, nil, template.WithEmpty, template.WithTitle, template.WithRootBlocks([]*model.Block{fileBlock}), template.WithAllBlocksEditsRestricted)
}
