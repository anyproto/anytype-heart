package report

import (
	"fmt"
	"testing"

	"github.com/anyproto/anytype-heart/core/block/importv2"
)

func TestZZProbe2RunWide(t *testing.T) {
	issues := []importv2.Issue{
		importv2.Warning(importv2.IssueDataLoss, "", "Notion's search stopped early"),
	}
	obj := Build("Import report", issues, 0, nil)
	for _, b := range obj.Payload.Blocks {
		if txt := b.GetText(); txt != nil {
			fmt.Printf("%-20s style=%v %q children=%v\n", b.Id, txt.Style, txt.Text, b.ChildrenIds)
		}
	}
	fmt.Println("=== mixed ===")
	issues2 := []importv2.Issue{
		importv2.Warning(importv2.IssueDataLoss, "", "Notion's search stopped early"),
		importv2.Warning(importv2.IssueDataLoss, "p1", "some block lost"),
	}
	obj2 := Build("Import report", issues2, 0, nil)
	for _, b := range obj2.Payload.Blocks {
		if txt := b.GetText(); txt != nil && (b.Id == "intro" || b.Id[:5] == "group") {
			fmt.Printf("%-20s style=%v %q children=%v\n", b.Id, txt.Style, txt.Text, b.ChildrenIds)
		}
	}
}
