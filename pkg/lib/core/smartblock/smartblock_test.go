package smartblock

import "testing"

func TestSmartBlockTypeFromID(t *testing.T) {
	type args struct {
		id string
	}
	tests := []struct {
		name    string
		args    args
		want    SmartBlockType
		wantErr bool
	}{
		{
			name:    "page",
			args:    args{"bafybat2vqhst2slonckga6f56ldytyrkzs2vj2k6g7zybxmgr4y5dsha"},
			want:    SmartBlockTypePage,
			wantErr: false,
		},
		{
			name:    "file",
			args:    args{"bafybeigwqj633cv7rrfirxvzwgvlcplgj2kbbi26d4ovq266vfhkrkaokm"},
			want:    SmartBlockTypeFile,
			wantErr: false,
		},
		{
			name:    "page2",
			args:    args{"bafybahy5zolqoeetiaf4kvjwbo3fq2hl4fnm55xq5tdpp5tr3ayg4oj5"},
			want:    SmartBlockTypePage,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SmartBlockTypeFromID(tt.args.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("SmartBlockTypeFromID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("SmartBlockTypeFromID() got = %v, want %v", got, tt.want)
			}
		})
	}
}
