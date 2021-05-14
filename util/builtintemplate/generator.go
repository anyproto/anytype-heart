package builtintemplate

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const prefix = "builtin:"

// GenerateTemplates fetch all templates with name starts with "builtin:" and generate go file
func (b *builtinTemplate) GenerateTemplates() (n int, err error) {
	recs, _, err := b.core.ObjectStore().QueryObjectInfo(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: "name",
				Condition:   model.BlockContentDataviewFilter_Like,
				Value:       pbtypes.String(prefix),
			},
		},
	}, []smartblock.SmartBlockType{
		smartblock.SmartBlockTypeTemplate,
	})
	if err != nil {
		return
	}
	var states []*state.State
	for _, rec := range recs {
		if err = b.blockService.Do(rec.Id, func(b sb.SmartBlock) error {
			st := b.NewState().Copy()
			name := pbtypes.GetString(st.Details(), "name")
			name = strings.TrimSpace(strings.ReplaceAll(name, prefix, ""))
			st.SetDetail("name", pbtypes.String(name))
			newId, err := threads.PatchSmartBlockType(st.RootId(), smartblock.SmartBlockTypeBundledTemplate)
			if err != nil {
				return err
			}
			st.SetRootId(newId)
			states = append(states, st)
			return nil
		}); err != nil {
			return
		}
	}
	if err = Generate("./util/builtintemplate/templates.gen.go", "builtintemplate", states); err != nil {
		return
	}
	return len(states), nil
}

func Generate(filename, pkgName string, states []*state.State) (err error) {
	const templateHeader = `// this is generated file, do not change
package %s

func init() {
	templatesBinary = append(templatesBinary,
`

	const templateFooter = `	)
}
`
	f, err := os.Create(filename)
	if err != nil {
		return
	}
	defer f.Close()
	wr := bufio.NewWriter(f)
	if _, err = fmt.Fprintf(wr, templateHeader, pkgName); err != nil {
		return
	}
	for _, st := range states {
		wr.WriteString("\t\t[]byte")
		stBytes, err := StateToBytes(st)
		if err != nil {
			return err
		}
		for _, b := range fmt.Sprint(stBytes) {
			switch b {
			case ' ':
				wr.WriteString(", ")
			case '[':
				wr.WriteString("{")
			case ']':
				wr.WriteString("}")
			default:
				wr.WriteRune(b)
			}
		}
		wr.WriteString(",\n")
	}
	if _, err = wr.WriteString(templateFooter); err != nil {
		return
	}
	if err = wr.Flush(); err != nil {
		return
	}
	return
}

func StateToBytes(s *state.State) (b []byte, err error) {
	snapshot := &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks:         s.BlocksToSave(),
			Details:        s.Details(),
			ExtraRelations: s.ExtraRelations(),
			ObjectTypes:    s.ObjectTypes(),
		},
	}
	for _, fk := range s.GetFileKeys() {
		snapshot.FileKeys = append(snapshot.FileKeys, &fk)
	}
	buf := bytes.NewBuffer(nil)
	wr := gzip.NewWriter(buf)
	data, err := snapshot.Marshal()
	if err != nil {
		return
	}
	if _, err = wr.Write(data); err != nil {
		return
	}
	if err = wr.Close(); err != nil {
		return
	}
	return buf.Bytes(), nil
}

func BytesToState(b []byte) (s *state.State, err error) {
	rd, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return
	}
	data, err := ioutil.ReadAll(rd)
	if err != nil {
		return
	}
	snapshot := &pb.ChangeSnapshot{}
	if err = snapshot.Unmarshal(data); err != nil {
		return
	}
	return state.NewDocFromSnapshot("", snapshot).(*state.State), nil
}
