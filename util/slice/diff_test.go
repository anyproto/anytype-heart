package slice

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Diff(t *testing.T) {
	origin := []string{"000", "001", "002", "003", "004", "005", "006", "007", "008", "009"}
	changed := []string{"000", "008", "001", "002", "003", "005", "006", "007", "009", "004"}

	chs := Diff(origin, changed)

	fmt.Println(chs)

	assert.Equal(t, chs, []Change{
		{Op: OperationRemove, Ids: []string{"004", "008"}},
		{Op: OperationAdd, Ids: []string{"008"}, AfterId: "000"},
		{Op: OperationAdd, Ids: []string{"004"}, AfterId: "009"}},
	)
}
