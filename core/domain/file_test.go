package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsFileId(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid file id",
			input: "bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku",
			want:  true,
		},
		{
			name:  "valid CID but not DagProtobuf",
			input: "bafyreiebxsn65332wl7qavcxxkfwnsroba5x5h2sshcn7f7cr66ztixb54",
			want:  false,
		},
		{
			name:  "empty",
			input: "",
			want:  false,
		},
		{
			name:  "invalid CID",
			input: "filecid",
			want:  false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsFileId(tc.input))
		})
	}
}
