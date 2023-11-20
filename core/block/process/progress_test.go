package process

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProgress_SetTotalPreservingRatio(t *testing.T) {
	newTotal := int64(256)

	t.Run("total sets as usual if done is zero", func(t *testing.T) {
		// done
		p := progress{doneCount: 0}

		// when
		p.SetTotalPreservingRatio(newTotal)

		//then
		assert.Equal(t, newTotal, p.totalCount)
	})

	t.Run("total sets as usual if doneCount is not less (>=) than totalCount", func(t *testing.T) {
		// done
		p := progress{doneCount: 100, totalCount: 30}

		// when
		p.SetTotalPreservingRatio(newTotal)

		//then
		assert.Equal(t, newTotal, p.totalCount)
	})

	t.Run("totalCount and doneCount are changed proportionally", func(t *testing.T) {
		// done
		oldDone := int64(30)
		oldTotal := int64(100)
		p := progress{doneCount: oldDone, totalCount: oldTotal}

		// when
		p.SetTotalPreservingRatio(newTotal)

		// then
		setTotal := p.totalCount
		setDone := p.doneCount

		// we introduce thresholds because division is involved in calculation
		// 2% of measuring value is a good approximation in mechanics: https://www.youtube.com/watch?v=vXjjNRSELTw
		threshold1 := 0.02 * float64(oldTotal*oldDone)
		threshold2 := 0.02 * float64(oldTotal)

		// 1. Ratio between totalCount and doneCount should be kept the same
		assert.True(t, math.Abs(float64(oldTotal*setDone)-float64(oldDone*setTotal)) < threshold1)
		// 2. setTotal - setDone = newTotal (function argument)
		assert.True(t, math.Abs(float64(setTotal-setDone-newTotal)) < threshold2)
	})
}
