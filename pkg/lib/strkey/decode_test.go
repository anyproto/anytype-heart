package strkey_test

import (
	"testing"

	. "github.com/anytypeio/go-anytype-middleware/pkg/lib/strkey"
	"github.com/stretchr/testify/assert"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		Name                string
		Address             string
		ExpectedVersionByte VersionByte
		ExpectedPayload     []byte
	}{
		{
			Name:                "AccountID",
			Address:             "P6NKHguuQnGraNM58WZTh2tPmRAnQQn1YzHQZYPmRt8WABDF",
			ExpectedVersionByte: 0xdd,
			ExpectedPayload: []byte{
				0x69, 0xa8, 0xc4, 0xcb, 0xb9, 0xf6, 0x4e, 0x8a,
				0x07, 0x98, 0xf6, 0xe1, 0xac, 0x65, 0xd0, 0x6c,
				0x31, 0x62, 0x92, 0x90, 0x56, 0xbc, 0xf4, 0xcd,
				0xb7, 0xd3, 0x73, 0x8d, 0x18, 0x55, 0xf3, 0x63,
			},
		},
		{
			Name:                "Seed",
			Address:             "SV8k5RKcUg1ZtN6qvcUGXfvqjv2nGeBhxy7sUnG1AxM5jB23",
			ExpectedVersionByte: 0xff,
			ExpectedPayload: []byte{
				0x69, 0xa8, 0xc4, 0xcb, 0xb9, 0xf6, 0x4e, 0x8a,
				0x07, 0x98, 0xf6, 0xe1, 0xac, 0x65, 0xd0, 0x6c,
				0x31, 0x62, 0x92, 0x90, 0x56, 0xbc, 0xf4, 0xcd,
				0xb7, 0xd3, 0x73, 0x8d, 0x18, 0x55, 0xf3, 0x63,
			},
		},
	}

	for _, kase := range cases {
		payload, err := Decode(kase.ExpectedVersionByte, kase.Address)
		if assert.NoError(t, err, "An error occured decoding case %s", kase.Name) {
			assert.Equal(t, kase.ExpectedPayload, payload, "Output mismatch in case %s", kase.Name)
		}
	}

	// the expected version byte doesn't match the actual version byte
	_, err := Decode(0xff, cases[0].Address)
	assert.Error(t, err)

	// invalid version byte
	_, err = Decode(VersionByte(2), cases[0].Address)
	assert.Error(t, err)

	// empty input
	_, err = Decode(0xdd, "")
	assert.Error(t, err)

	// corrupted checksum
	_, err = Decode(0xff, "ST1JhsR34aiso4N5RgiM2F7XRSxihACsrTDSm64VfnGTfeop")
	assert.Error(t, err)

	// corrupted payload
	_, err = Decode(0xdd, "P6NKHguuQnGraNM58WZTh2tPmRAnQQn1YzHQZYPmRt8WALT5")
	assert.Error(t, err)
}
