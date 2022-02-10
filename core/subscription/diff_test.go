package subscription

import (
	"fmt"
	"strings"
	"testing"
)

func TestListDiff(t *testing.T) {
	var before = []string{"1", "2", "3"}
	var after = []string{"0", "1", "2"}
	d := &listDiff{}
	d.reset()
	for _, id := range before {
		d.fillAfter(id)
	}
	d.reset()
	for _, id := range after {
		d.fillAfter(id)
	}
	ctx := &opCtx{}
	d.diff(ctx, "", nil)
	for _, ch := range ctx.change {
		t.Log("ch", ch)
	}
	for _, rm := range ctx.remove {
		t.Log("rm", rm.id)
	}
}

func BenchmarkDiff(b *testing.B) {
	b.ReportAllocs()
	genData := func(n int, reverse bool) []string {
		res := make([]string, n)
		for i := 0; i < n; i++ {
			if reverse {
				res[i] = strings.Repeat("x", 40) + fmt.Sprint(n-i)
			} else {
				res[i] = strings.Repeat("x", 40) + fmt.Sprint(i)
			}
		}
		return res
	}
	benchmark := func(before, after []string) func(b *testing.B) {
		d := &listDiff{}
		d.reset()
		for _, id := range before {
			d.fillAfter(id)
		}
		d.reset()
		ctx := &opCtx{}
		return func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for _, id := range after {
					d.fillAfter(id)
				}
				d.diff(ctx, "", nil)
				d.reset()
				ctx.reset()
				after, before = before, after
			}
		}
	}
	var before, after []string
	for _, n := range []int{
		100, 1000, 10000, 100000,
	} {
		b.Run(fmt.Sprintf("big-diff-%d", n), benchmark(genData(n, false), genData(n, true)))
		before = genData(n, false)
		after = genData(n, false)
		after[(n/2)-1], after[(n/2)] = after[(n/2)], after[(n/2)-1]
		b.Run(fmt.Sprintf("small-diff-%d", n), benchmark(before, after))
		before = genData(n, false)
		b.Run(fmt.Sprintf("equal-%d", n), benchmark(before, before))
	}

}
