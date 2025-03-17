package testutil

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func AssertProtosEqual(t *testing.T, x, y any) {
	if diff := cmp.Diff(x, y, protocmp.Transform()); diff != "" {
		t.Errorf("unexpected difference:\n%v", diff)
	}
}
