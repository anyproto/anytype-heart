package markdown

import (
	"fmt"
	"testing"

	"github.com/anyproto/anytype-heart/core/block/importv2/report"
)

func TestProbeReportExplosion(t *testing.T) {
	files := map[string]string{}
	const n = 300
	for i := 0; i < n; i++ {
		files[fmt.Sprintf("page%03d.md", i)] = fmt.Sprintf("# Page %d\n\n![](missing%03d.png)\n", i, i)
	}
	sink, _, _ := runConverter(t, files)
	t.Logf("issues: %d", len(sink.issues))
	msgs := map[string]int{}
	for _, is := range sink.issues {
		msgs[is.Message]++
	}
	t.Logf("distinct messages: %d", len(msgs))
	for m, c := range msgs {
		if c > 5 || len(msgs) < 5 {
			t.Logf("  %dx %q", c, m)
		}
	}
	obj := report.Build("Import report", sink.issues, 0, func(k string) report.Source {
		return report.Source{Name: k, Resolved: true}
	})
	blocks := obj.Payload.Blocks
	rows := 0
	for _, b := range blocks {
		if b.GetTableRow() != nil {
			rows++
		}
	}
	t.Logf("report blocks: %d, table rows: %d", len(blocks), rows)
}
