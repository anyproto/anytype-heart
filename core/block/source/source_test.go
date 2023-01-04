package source

import (
	"fmt"
	"os"
	"testing"
)

func Test_snapshotChance(t *testing.T) {
	if os.Getenv("ANYTYPE_TEST_SNAPSHOT_CHANCE") == "" {
		t.Skip()
		return
	}
	for i := 0; i <= 500; i++ {
		for s := 0; s <= 10000; s++ {
			if snapshotChance(s) {
				fmt.Println(s)
				break
			}
		}
	}
	fmt.Println()
	// here is an example of distribution histogram
	// https://docs.google.com/spreadsheets/d/1xgH7fUxno5Rm-0VEaSD4LsTHeGeUXQFmHsOm29M6paI
}

func Test_snapshotChance2(t *testing.T) {
	if os.Getenv("ANYTYPE_TEST_SNAPSHOT_CHANCE") == "" {
		t.Skip()
		return
	}
	for s := 0; s <= 10000; s++ {
		total := 0
		for i := 0; i <= 50000; i++ {
			if snapshotChance(s) {
				total++
			}
		}
		fmt.Printf("%d\t%.5f\n", s, float64(total)/50000)
	}

	// here is an example of distribution histogram
	// https://docs.google.com/spreadsheets/d/1xgH7fUxno5Rm-0VEaSD4LsTHeGeUXQFmHsOm29M6paI
}
