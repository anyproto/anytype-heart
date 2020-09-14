package strkey_test

import (
	"testing"

	. "github.com/anytypeio/go-anytype-middleware/pkg/lib/strkey"
	"github.com/stretchr/testify/assert"
)

func TestVersion(t *testing.T) {
	cases := []struct {
		Name                string
		Address             string
		ExpectedVersionByte VersionByte
	}{
		{
			Name:                "AccountID",
			Address:             "P46vw5b3M6qjFsnWVSCsPusZsypRPeKTSzZ9RHjbTXedMdR6",
			ExpectedVersionByte: 0xdd,
		},
		{
			Name:                "Seed",
			Address:             "SUMgBQ377QKBnYfKuvToBS3gPFjzWicmhykQvoTJK9LNySu8",
			ExpectedVersionByte: 0xff,
		},
		{
			Name:                "Other (0x60)",
			Address:             "Ac99rdvmhNPWhzx6wsTySWXJ5yt9HZhNaS8b8EQVHHSo5Wje",
			ExpectedVersionByte: VersionByte(0x60),
		},
	}

	for _, kase := range cases {
		actual, err := Version(kase.Address)
		if assert.NoError(t, err, "An error occured decoding case %s", kase.Name) {
			assert.Equal(t, kase.ExpectedVersionByte, actual, "Output mismatch in case %s", kase.Name)
		}
	}
}
