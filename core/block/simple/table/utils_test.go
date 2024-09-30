package table

import "testing"

func TestIsTableCell(t *testing.T) {
	tests := []struct {
		blockId string
		want    bool
	}{
		{"-", false},
		{"--", false},
		{"-a-", false},
		{"a--b", false},
		{"-a", false},
		{"b-", false},
		{"abc-xyz", true},
		{"-abc-xyz", false},
		{"abc-xyz-", false},
		{"", false},
		{"abc", false},
		{"abc-xyz-def", false},
	}

	for _, tt := range tests {
		t.Run(tt.blockId, func(t *testing.T) {
			got := IsTableCell(tt.blockId)
			if got != tt.want {
				t.Errorf("IsTableCell(%q) = %v, want %v", tt.blockId, got, tt.want)
			}
		})
	}
}
