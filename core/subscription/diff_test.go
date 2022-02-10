package subscription

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestListDiffFuzz(t *testing.T) {
	var genRandomUniqueSeq = func(l int) []string {
		ids := map[string]struct{}{}
		for len(ids) < l {
			ids[fmt.Sprint(rand.Intn(int(float64(l)*1.2)))] = struct{}{}
		}
		res := make([]string, 0, l)
		for id := range ids {
			res = append(res, id)
		}
		return res
	}

	var chToString = func(ch opChange) string {
		if ch.isAdd {
			return fmt.Sprintf("add: %s after: %s", ch.id, ch.afterId)
		}
		return fmt.Sprintf("move: %s after: %s", ch.id, ch.afterId)
	}

	var checkBeforeAfter = func(before, after []string) {
		var debug []string

		d := newListDiff(before)
		for _, id := range after {
			d.fillAfter(id)
		}
		ctx := &opCtx{}
		d.diff(ctx, "", nil)

		var resAfter = make([]string, len(before))
		copy(resAfter, before)

		for i, ch := range ctx.change {
			if !ch.isAdd {
				resAfter = slice.Remove(resAfter, ch.id)
			}
			if ch.afterId == "" {
				resAfter = append([]string{ch.id}, resAfter...)
			} else {
				pos := slice.FindPos(resAfter, ch.afterId)
				resAfter = slice.Insert(resAfter, pos+1, ch.id)
			}
			debug = append(debug, fmt.Sprintf("%d:\t %+v", i, chToString(ch)))
			debug = append(debug, fmt.Sprintf("%d:\t %v", i, resAfter))
		}
		for _, rm := range ctx.remove {
			resAfter = slice.Remove(resAfter, rm.id)
		}
		ok := assert.ObjectsAreEqual(after, resAfter)

		if !ok {
			t.Log("after", after)
			t.Log("afterRes", resAfter)
			t.Log("before", before)

			for _, dbg := range debug {
				t.Log(dbg)
			}
		} else {
			//t.Logf("ch: %d; rm: %d; %v", len(ctx.change), len(ctx.remove), resAfter)
		}
		assert.True(t, ok)
		return
	}

	rand.Seed(time.Now().UnixNano())

	var initialLen = 5

	var before, after []string
	before = genRandomUniqueSeq(initialLen)
	for i := 0; i < 20; i++ {
		after = genRandomUniqueSeq(initialLen + rand.Intn(1+i))
		checkBeforeAfter(before, after)
		before = after
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
		d := newListDiff(before)
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
