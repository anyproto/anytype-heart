package report

import (
	"fmt"
	"testing"

	"github.com/anyproto/anytype-heart/core/block/importv2"
)

func TestZZProbeRunWide(t *testing.T) {
	issues := []importv2.Issue{
		importv2.Warning(importv2.IssueDataLoss, "", "Notion's search stopped early"),
	}
	obj := Build("Import report", issues, 0, nil)
	for _, b := range obj.Payload.Blocks {
		if txt := b.GetText(); txt != nil {
			fmt.Printf("%-20s %q children=%v\n", b.Id, txt.Text, b.ChildrenIds)
		}
	}
}

func TestZZProbeEmpty(t *testing.T) {
	obj := Build("Import report", nil, 0, nil)
	fmt.Println("blocks:", len(obj.Payload.Blocks))
}
